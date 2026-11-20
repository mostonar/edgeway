"""
model_codec.py — codec for carrying FL model weights in oneM2M contentInstances

[Why it is needed]
The previous implementation put only a local file path (model_path) in the contentInstance con,
and the actual PyTorch weights were exchanged through the file system of the same PC.
So (1) it broke immediately once an MN-AE ran on a different machine, and
(2) the claim "federated learning runs over oneM2M resources" did not hold.
(oneM2M was only carrying control messages and metadata)

[What it does]
state_dict -> torch.save serialization -> (optional) fp16 reduction -> gzip -> base64 string.
The string is put in con and sent through the CSE. The receiver restores it in reverse order
and verifies integrity with sha256.

[Size and CSE limits]
Based on CVAE1D (latent 24, hidden 48, seq_len 60) = 90,806 parameters
    fp32 as is    : base64 about 474 KB
    fp16 + gzip   : base64 about 217 KB   <- default

ACME has no small payload limit, so it fits in one CIN as is. But since it may move to
a CSE with a small limit, both paths are supported.

  (1) Single CIN  — if the base64 is under the limit, put it in con as is
  (2) Chunking    — if over the limit, split it into a dedicated child container,
                  and the main CIN carries a manifest with only the chunk count and hash

Chunks go into a child container (cnt-weights), so they do not trigger the subscription
(CREATE_OF_DIRECT_CHILD) on the parent container. Dozens of chunks therefore do not
flood notifications; only the single CIN holding the manifest fires a notification.
Each chunk CIN has an explicit rn (w{round}_{0000}), so the receiver can RETRIEVE it
by name without discovery.

[About fp16]
Sending weights as half precision is a common quantization compression in FL.
Relative error is around 4e-04, which is buried in the AE reconstruction error; if that matters,
turn it off with config.FL_WEIGHTS_FP16 = False (transfer size doubles).
"""

import base64
import gzip
import hashlib
import io
from typing import Any, Callable, Dict, List, Optional, Tuple

import torch

CODEC_FP16_GZIP = "fp16+gzip+b64"
CODEC_FP32_GZIP = "fp32+gzip+b64"

# The con of one chunk CIN is JSON of the form {"i":index,"d":"chunk"}.
# Headroom reserved for extra characters such as the index, quotes and braces.
CHUNK_OVERHEAD_BYTES = 2048


def encode_state_dict(
    state_dict: Dict[str, Any],
    fp16: bool = True,
    gzip_level: int = 6,
) -> Dict[str, Any]:
    """
    Turn a state_dict into a base64 string that fits in a contentInstance.

    The return value is a dict that can be spread directly into con:
        weights_b64    : base64 string (the actual weights)
        weights_codec  : identifier for how to restore
        weights_sha256 : hash of the compressed bytes (for integrity check)
        weights_bytes  : length of the base64 string (for the size guard)
    """
    if fp16:
        payload_sd = {
            k: (v.half() if torch.is_tensor(v) and v.dtype == torch.float32 else v)
            for k, v in state_dict.items()
        }
    else:
        payload_sd = state_dict

    buf = io.BytesIO()
    torch.save(payload_sd, buf)
    raw = gzip.compress(buf.getvalue(), gzip_level)
    b64 = base64.b64encode(raw).decode("ascii")

    return {
        "weights_b64": b64,
        "weights_codec": CODEC_FP16_GZIP if fp16 else CODEC_FP32_GZIP,
        "weights_sha256": hashlib.sha256(raw).hexdigest(),
        "weights_bytes": len(b64),
    }


def split_for_cin(
    enc: Dict[str, Any],
    max_cin_bytes: int,
    overhead: int = CHUNK_OVERHEAD_BYTES,
) -> Tuple[List[str], Dict[str, Any]]:
    """
    Split the encode_state_dict result to fit the CIN size limit.

    Returns:
        chunks   : list of base64 chunk strings (order is the chunk index)
        manifest : metadata for the main CIN. weights_b64 is removed and
                   weights_chunked / weights_chunk_count are added instead.
                   Which container the chunks went into and under what names
                   is filled in by the caller, who knows the paths.
    """
    b64 = enc["weights_b64"]
    size = max(1024, int(max_cin_bytes) - int(overhead))

    chunks = [b64[i:i + size] for i in range(0, len(b64), size)]

    manifest = {
        "weights_chunked": True,
        "weights_chunk_count": len(chunks),
        "weights_chunk_size": size,
        "weights_codec": enc["weights_codec"],
        "weights_sha256": enc["weights_sha256"],
        "weights_bytes": enc["weights_bytes"],
    }
    return chunks, manifest


def decode_state_dict(
    meta: Dict[str, Any],
    fetch_chunk: Optional[Callable[[int], Optional[str]]] = None,
) -> Optional[Dict[str, Any]]:
    """
    Restore a state_dict from con (or part of it).

    Handles three cases.
      - If weights_b64 is present, restore it directly (single CIN)
      - If weights_chunked is True, fetch chunks with fetch_chunk(i), join them, then restore
      - If neither, None. No weights were carried, so the caller treats it as a failure.

    fetch_chunk is a function that takes a chunk index and returns the base64 chunk string.
    How the receiver fetches it from the CSE is up to the caller (this module knows nothing about networking).
    """
    b64 = meta.get("weights_b64")

    if not b64 and meta.get("weights_chunked"):
        if fetch_chunk is None:
            raise ValueError(
                "weights are chunked but fetch_chunk is missing"
            )

        count = int(meta.get("weights_chunk_count", 0))
        if count <= 0:
            raise ValueError("weights_chunk_count is invalid")

        parts = []
        for i in range(count):
            part = fetch_chunk(i)
            if not part:
                raise ValueError(
                    f"failed to fetch weight chunk {i}/{count}"
                )
            parts.append(part)
        b64 = "".join(parts)

    if not b64:
        return None

    # Verify the transmitted bytes (compressed) as they are.
    # The encoding side hashes the same bytes, so the two values must match.
    compressed = base64.b64decode(b64)

    expected = meta.get("weights_sha256")
    if expected:
        actual = hashlib.sha256(compressed).hexdigest()
        if actual != expected:
            raise ValueError(
                f"weights sha256 mismatch: expected {expected[:12]}…, "
                f"got {actual[:12]}…"
            )

    raw = gzip.decompress(compressed)

    try:
        sd = torch.load(io.BytesIO(raw), map_location="cpu", weights_only=True)
    except TypeError:
        # Older torch without the weights_only argument
        sd = torch.load(io.BytesIO(raw), map_location="cpu")

    # If sent as fp16, convert back to fp32 for training/aggregation
    if meta.get("weights_codec") == CODEC_FP16_GZIP:
        sd = {
            k: (v.float() if torch.is_tensor(v) and v.dtype == torch.float16 else v)
            for k, v in sd.items()
        }
    return sd


def chunk_resource_name(round_num: int, index: int) -> str:
    """Resource name of a chunk CIN, so the receiver can RETRIEVE it directly without discovery."""
    return f"w{int(round_num)}_{int(index):04d}"


def describe(meta: Dict[str, Any]) -> str:
    """One-line summary for logs."""
    n = meta.get("weights_bytes", 0)
    codec = meta.get("weights_codec", "?")
    if meta.get("weights_chunked"):
        return (
            f"{codec} {n / 1024:.0f}KB "
            f"({meta.get('weights_chunk_count')} chunks)"
        )
    return f"{codec} {n / 1024:.0f}KB (single CIN)"
