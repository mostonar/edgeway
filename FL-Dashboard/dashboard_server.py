"""
FL-Dashboard/dashboard_server.py — FL dashboard backend

Background threads:

1. fl-poll
   - oneM2M status polling
   - detect new global models
   - recompute the validation threshold
   - recompute the test stream reconstruction score

2. score-stream
   - send test sample reconstruction errors via SSE
   - score_idx is preserved even when the global model updates
   - MN3 shows only normal data before the configured round
   - shows only anomalous data from the configured round

SSE events:

  init
    current state right after connecting

  round
    FL state and round changes

  threshold
    threshold recomputed for the global model

  score
    per-node reconstruction error

  summary
    final evaluation result

Reads from:

  Central IN-CSE    round state (cnt-fl-control), per-site summary (loss and sample count from the drop box),
                 global model (cnt-global-model), per-round evaluation (cnt-fl-inference)
  Site MN-CSE       per-window anomaly scores (TRAINING-AE-i/cnt-window-scores).
                 These are site detection results and are not uploaded to the IN-CSE, so they are read on the site side.

So this dashboard runs on a host that can reach the site MN-CSEs (the Edge VM in the current deployment).

Run:

  bash FL-Dashboard/run_dashboard.sh

Address:

  http://localhost:7000
"""

from __future__ import annotations

import json
import os
import pickle
import queue
import threading
import time
from pathlib import Path

import numpy as np
import torch
from flask import Flask, Response, send_from_directory
from sklearn.metrics import roc_auc_score

import config
import onem2m_utils as om2m
import model_codec
from model_cvae import CVAE1D


# ════════════════════════════════════════════════════════
# Defaults
# ════════════════════════════════════════════════════════

PORT = int(
    os.getenv(
        "FL_DASHBOARD_PORT",
        "7000",
    )
)
POLL_INTERVAL = float(
    os.getenv(
        "FL_DASHBOARD_POLL_INTERVAL",
        "3.0",
    )
)
# Watch only the round state (cnt-fl-control) separately and more often. One full polling cycle that includes
# node uploads (large CINs carrying weights) and model re-scoring takes several seconds,
# so when tied to it the screen lagged behind even after the IN-AE opened the next round.
CONTROL_POLL_INTERVAL = float(
    os.getenv(
        "FL_DASHBOARD_CONTROL_INTERVAL",
        "1.0",
    )
)
SCORE_INTERVAL = float(
    os.getenv(
        "FL_DASHBOARD_SCORE_INTERVAL",
        "0.5",
    )
)
N_SIGMA = float(
    getattr(
        config,
        "THRESHOLD_N_SIGMA",
        3.0,
    )
)
K_CONSECUTIVE = int(
    getattr(
        config,
        "ANOMALY_K_CONSECUTIVE",
        3,
    )
)
ANOMALY_NODE = str(
    getattr(
        config,
        "ANOMALY_DEMO_NODE",
        "mn3",
    )
)
ANOMALY_START_ROUND = int(
    getattr(
        config,
        "ANOMALY_START_ROUND",
        7,
    )
)
# The screen file (fl_dashboard.html) and vendor/ go in the same folder as this file.
PROJECT_ROOT = Path(
    __file__
).resolve().parent
HTML_PATH = (
    PROJECT_ROOT
    / "fl_dashboard.html"
)
PKL_DIR = Path(
    getattr(
        config,
        "FL_PKL_DIR",
        "/tmp/fl_data/amr",
    )
)
NODES = [
    "mn1",
    "mn2",
    "mn3",
]

app = Flask(
    __name__,
)


# ════════════════════════════════════════════════════════
# SSE subscribers
# ════════════════════════════════════════════════════════

_subscribers: list[queue.Queue] = []
_subscribers_lock = threading.Lock()


# ════════════════════════════════════════════════════════
# Shared state
# ════════════════════════════════════════════════════════

_shared: dict = {
    "fl_state": "FL_READY",
    "round": 0,
    "max_rounds": config.GLOBAL_ROUNDS,

    # node → local training metrics
    "nodes": {},

    # node → threshold
    "thresholds": {},

    # node → reconstruction scores
    "scores": {},

    # node → test labels
    "labels": {},

    # Stream position preserved even when the global model changes
    "score_idx": 0,

    # Global model round currently reflected by the dashboard
    "model_round": -1,

    # FL round whose CSE drop-box scores have been reflected.
    # A marker for updating panels every round even without a global model.
    "scores_round": -1,

    "summary_sent": False,

    "anomaly_start_round": ANOMALY_START_ROUND,

    # Before this round, no anomaly is shown on screen either.
    # Uses the same value as the criterion that blocks verdicts (CINs) on the MN side. If the two disagree,
    # the screen turns red while there is nothing on the CSE.
    "verdict_start_round": int(
        getattr(config, "VERDICT_START_ROUND", 1)
    ),

    # Per-round fixed validation set evaluation results. Kept so that clients connecting later
    # also receive past rounds.
    "evals": [],
}

# How many rounds of evaluation log to hold. Set comfortably above the round count.
EVAL_HISTORY_MAX = 60

_shared_lock = threading.Lock()


# ════════════════════════════════════════════════════════
# SSE helpers
# ════════════════════════════════════════════════════════

def _broadcast(event: dict) -> None:
    """
    Send an event to every currently connected SSE client.
    """

    message = (
        "data: "
        + json.dumps(
            event,
            ensure_ascii=False,
        )
        + "\n\n"
    )

    with _subscribers_lock:
        disconnected = []

        for subscriber in _subscribers:
            try:
                subscriber.put_nowait(
                    message,
                )
            except queue.Full:
                disconnected.append(
                    subscriber,
                )
        for subscriber in disconnected:
            if subscriber in _subscribers:
                _subscribers.remove(
                    subscriber,
                )


# ════════════════════════════════════════════════════════
# HTTP Endpoint
# ════════════════════════════════════════════════════════

@app.route("/vendor/<path:name>")
def vendor(name: str):
    """
    Serve the library from inside the repo instead of an external CDN.

    Loading Chart.js from cdnjs blocks parsing in <head>, so when opened from off campus
    the screen stays blank until it finishes downloading. If the network is blocked it waits
    until the timeout. It is served from here so the demo does not depend on the outside network.
    """
    return send_from_directory(PROJECT_ROOT / "vendor", name,
                               max_age=86400)


@app.route("/")
def index():
    if not HTML_PATH.exists():
        return (
            f"Dashboard HTML not found: {HTML_PATH}<br>"
            "프로젝트 루트에 "
            "fl_dashboard.html 파일이 있는지 확인하세요.",
            500,
        )

    return HTML_PATH.read_text(
        encoding="utf-8",
    )


@app.route("/health")
def health():
    pkl_files = {
        node: (
            PKL_DIR
            / f"{node}.pkl"
        ).exists()
        for node in NODES
    }

    with _shared_lock:
        state = {
            "ok": True,

            "oneM2M": config.BASE_URL,

            "state": _shared["fl_state"],
            "round": _shared["round"],
            "max_rounds": _shared["max_rounds"],
            "model_round": _shared["model_round"],

            "html_path": str(
                HTML_PATH,
            ),
            "html_exists": HTML_PATH.exists(),

            "pkl_dir": str(
                PKL_DIR,
            ),
            "pkl_dir_exists": PKL_DIR.exists(),
            "pkl_files": pkl_files,

            "nodes": _shared["nodes"],
            "thresholds": _shared["thresholds"],
            "score_idx": _shared["score_idx"],
            "summary_sent": _shared["summary_sent"],

            "anomaly_node": ANOMALY_NODE,
            "verdict_start_round": _shared["verdict_start_round"],
            "anomaly_start_round": (
                ANOMALY_START_ROUND
            ),
        }

    return state


@app.route("/stream")
def stream():
    subscriber: queue.Queue = queue.Queue(
        maxsize=150,
    )
    with _subscribers_lock:
        _subscribers.append(
            subscriber,
        )

    # Send the current state right after connecting
    with _shared_lock:
        snapshot = {
            "type": "init",
            "fl_state": _shared["fl_state"],
            "round": _shared["round"],
            "max_rounds": _shared["max_rounds"],
            "nodes": _shared["nodes"],
            "thresholds": _shared["thresholds"],
            "model_round": _shared["model_round"],
            # Also send evaluations of rounds that finished before connecting.
            "evals": list(_shared["evals"]),
        }

    try:
        subscriber.put_nowait(
            "data: "
            + json.dumps(
                snapshot,
                ensure_ascii=False,
            )
            + "\n\n"
        )
    except queue.Full:
        pass

    def generate():
        try:
            while True:
                try:
                    yield subscriber.get(
                        timeout=25,
                    )
                except queue.Empty:
                    yield ": heartbeat\n\n"

        except GeneratorExit:
            pass

        finally:
            with _subscribers_lock:
                if subscriber in _subscribers:
                    _subscribers.remove(
                        subscriber,
                    )

    return Response(
        generate(),
        mimetype="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",
        },
    )


# ════════════════════════════════════════════════════════
# Start time of the current demo run
# ════════════════════════════════════════════════════════

# ════════════════════════════════════════════════════════
# Find the latest global model
# ════════════════════════════════════════════════════════

def _find_latest_model(
    max_round: int | None = None,
) -> tuple[int, Path | None]:
    """
    Get the most recent global model from cnt-global-model on the IN-CSE.
    Models from rounds later than the current FL round are excluded.
    Returns: (round, state_dict) or (-1, None)
    """

    # The IN-AE distributes the global model in con via cnt-global-model.
    # The dashboard receives it from there. It runs on a different host from the IN-AE,
    # so it does not look at the IN-AE's local disk (/tmp/fl_models/global).
    return _latest_global_from_cse(max_round)


def _latest_global_from_cse(max_round: int | None = None):
    """
    Fetch the latest global model from cnt-global-model on the central CSE.

    Returns: (round, state_dict) or (-1, None)

    con carries the weights as fp16+gzip+base64. If split into chunks,
    model_codec fetches the rest via fetch_chunk.
    """
    path = om2m.in_path(f"{config.IN_AE_NAME}/cnt-global-model")
    try:
        meta = _parse_content_instance(
            om2m.get_latest_content_instance(path)
        )
    except Exception:
        return -1, None

    if not meta or meta.get("type") != "global-model":
        return -1, None

    try:
        model_round = int(meta.get("global_round", -1))
    except (TypeError, ValueError):
        return -1, None

    if max_round is not None and model_round > max_round:
        return -1, None

    def fetch_chunk(name: str):
        return _parse_content_instance(
            om2m.get_latest_content_instance(
                f"{meta.get('weights_chunk_container', '')}/{name}"
            )
        )

    try:
        state = model_codec.decode_state_dict(meta, fetch_chunk=fetch_chunk)
    except Exception as e:
        print(f"  ⚠ 글로벌 모델 복원 실패 ({type(e).__name__}: {e})")
        return -1, None

    return (model_round, state) if state is not None else (-1, None)


# ════════════════════════════════════════════════════════
# Load the model
# ════════════════════════════════════════════════════════

def _load_model(
    model_path: str | Path,
) -> CVAE1D:
    ae_config = config.AE_CFG

    # Omitting cond_dim builds it with the CVAE1D default (11). The actual setting is 8, so
    # encoder.0.weight gets 49 input channels instead of 46 and loading fails.
    model = CVAE1D(
        n_channels=ae_config.n_channels,
        cond_dim=ae_config.cond_dim,
        latent_dim=ae_config.latent_dim,
        seq_len=ae_config.seq_len,
    )

    # If it came from the CSE it is already a state_dict. If it is a file path, read it.
    state_dict = (
        model_path
        if isinstance(model_path, dict)
        else torch.load(model_path, map_location="cpu")
    )

    model.load_state_dict(
        state_dict,
    )

    model.eval()

    return model


# ════════════════════════════════════════════════════════
# Reconstruction score computation
# ════════════════════════════════════════════════════════

def _compute_scores(
    model: CVAE1D,
    signals: np.ndarray,
    batch_size: int = 64,
) -> np.ndarray:
    """
    signals shape:
      (N, sequence_length)

    return shape:
      (N,)
    """

    if len(signals) == 0:
        return np.array(
            [],
            dtype=np.float32,
        )

    score_batches: list[np.ndarray] = []

    tensor = torch.tensor(
        signals,
        dtype=torch.float32,
    )

    with torch.no_grad():
        for start in range(
            0,
            len(tensor),
            batch_size,
        ):
            batch = tensor[
                start:start + batch_size
            ].unsqueeze(1)

            reconstruction = model(
                batch,
            )

            error = (
                (
                    batch
                    - reconstruction
                )
                ** 2
            ).mean(
                dim=(1, 2),
            )
            score_batches.append(
                error.cpu().numpy(),
            )

    return np.concatenate(
        score_batches,
    )


# ════════════════════════════════════════════════════════
# Threshold computation
# ════════════════════════════════════════════════════════

def _compute_threshold(
    model: CVAE1D,
    validation_signals: np.ndarray,
    validation_labels: np.ndarray,
) -> float:
    normal_signals = validation_signals[
        validation_labels == 0
    ]

    if len(normal_signals) == 0:
        return 1.0

    scores = _compute_scores(
        model,
        normal_signals,
    )

    return float(
        scores.mean()
        + N_SIGMA * scores.std()
    )


# ════════════════════════════════════════════════════════
# Recompute scores when the global model changes
# ════════════════════════════════════════════════════════

_mn_lock = threading.Lock()


def _mn_endpoint_of(node: str):
    """
    MN-CSE the site app is currently attached to (base, cse).

    Follows mn_base / mn_cse from the drop-box uploads. After a CSE handover (scenario 6)
    it still reads the CSE the site app moved to. If there is no upload yet, the rule-based address.
    """
    with _shared_lock:
        data = dict(_shared["nodes"].get(node) or {})
    if data.get("mn_base") and data.get("mn_cse"):
        return data["mn_base"], data["mn_cse"]
    return config.mn_endpoint(node)


def _node_scores_from_cse(node: str):
    """
    Read a node's per-window scores from the site MN-CSE.

    Each round the site app writes window_scores / window_labels / threshold to
    TRAINING-AE-i/cnt-window-scores. These are site detection results and are not uploaded
    to the IN-CSE. In CSE mode there are no PKL files, so this is used.

    Returns: (scores, labels, threshold) or None
    """
    base, cse = _mn_endpoint_of(node)
    path = (f"{cse}/{config.training_ae_name(node)}"
            f"/{config.WINDOW_SCORES_CNT}")

    try:
        # om2m has a single site-side connection, so lock while switching sites to read.
        # Other threads use only central (@in/) paths, so they are unaffected.
        with _mn_lock:
            om2m.set_endpoint(base, cse)
            content = _parse_content_instance(
                om2m.get_latest_content_instance(path)
            )
    except Exception as error:
        print(f"    ⚠ {node} 윈도우 점수 조회 실패 ({base}/{path}): {error}")
        return None

    if not content:
        return None

    # Containers are not cleared between runs. CINs uploaded before this dashboard started
    # are the previous run's last-round scores, and using them made the chart draw early
    # at round 0.
    if _is_stale_payload(content):
        return None

    raw = content.get("window_scores")
    if not raw:
        return None

    scores = np.asarray(raw, dtype=np.float32)

    raw_labels = content.get("window_labels")
    if raw_labels is not None and len(raw_labels) == len(scores):
        labels = np.asarray(raw_labels, dtype=np.int64)
    else:
        # In experiments with anomaly injection off, all labels are normal.
        labels = np.zeros(len(scores), dtype=np.int64)

    threshold = content.get("threshold")
    try:
        threshold = float(threshold) if threshold is not None else None
    except (TypeError, ValueError):
        threshold = None

    return scores, labels, threshold


def _reload_scores_if_needed() -> None:
    """
    When a new global model is created:

    1. recompute the validation threshold
    2. recompute the test stream reconstruction score
    3. keep the existing score_idx

    score_idx is not reset to 0 as in the old code.
    """

    with _shared_lock:
        current_model_round = _shared[
            "model_round"
        ]
        current_fl_round = _shared[
            "round"
        ]

    model_round, model_path = _find_latest_model(
        max_round=current_fl_round,
    )

    with _shared_lock:
        current_scores_round = _shared["scores_round"]
        # Last round uploaded per node. If it changes, new scores have arrived.
        uploads = tuple(sorted(
            (node, data.get("round", 0))
            for node, data in _shared["nodes"].items()
        ))
        last_uploads = _shared.get("scores_uploads")

    has_new_model = (
        model_round > current_model_round
        and model_path is not None
    )
    # Even without a global model yet (Round 1), there are scores uploaded by the MN-AE.
    # Looking only at the round number, round N's scores were not read until N+1 started even after every node
    # had uploaded, so the chart lagged by a round. Read whenever the uploads change too.
    has_new_round = (current_fl_round > current_scores_round
                     or uploads != last_uploads)

    if not has_new_model and not has_new_round:
        return

    model = None

    if has_new_model:
        print(
            "  [Scores] "
            f"새 글로벌 모델 R{model_round} 감지"
        )
        try:
            model = _load_model(
                model_path,
            )
        except Exception as error:
            print(
                "  ✗ 글로벌 모델 로드 실패: "
                f"{error}"
            )
            model = None

    if model is None and not has_new_round:
        return

    if model is None:
        print(
            "  [Scores] "
            f"R{current_fl_round} — 글로벌 모델 없이 "
            "사업장 MN-CSE 점수로 갱신"
        )

    new_thresholds: dict[str, float] = {}
    new_scores: dict[str, np.ndarray] = {}
    new_labels: dict[str, np.ndarray] = {}

    for node in NODES:
        pkl_path = (
            PKL_DIR
            / f"{node}.pkl"
        )

        # Scoring directly from PKL requires a global model.
        if not pkl_path.exists() or model is None:
            # CSE mode. Use the scores the site app wrote to the MN-CSE as is.
            from_cse = _node_scores_from_cse(node)

            if from_cse is None:
                print(
                    f"  ⚠ {node}: PKL 도 CSE 점수도 없다 "
                    f"({pkl_path.name})"
                )
                continue

            cse_scores, cse_labels, cse_threshold = from_cse

            if cse_threshold is None:
                # If no threshold was received, compute a provisional one from the received scores.
                # Can happen in early rounds before the model converges.
                cse_threshold = float(
                    cse_scores.mean()
                    + config.THRESHOLD_N_SIGMA * cse_scores.std()
                )

            new_scores[node] = cse_scores
            new_labels[node] = cse_labels
            new_thresholds[node] = round(cse_threshold, 4)

            print(
                f"    {node}: n={len(cse_scores)}, "
                f"threshold={cse_threshold:.4f} (CSE 드롭박스)"
            )
            continue

        try:
            with pkl_path.open(
                "rb",
            ) as file:
                dataset = pickle.load(
                    file,
                )
        except Exception as error:
            print(
                f"  ✗ {node} PKL 로드 실패: "
                f"{error}"
            )
            continue

        sequence_length = (
            config.AE_CFG.seq_len
        )
        empty_signals = np.empty(
            (
                0,
                sequence_length,
            ),
            dtype=np.float32,
        )
        empty_labels = np.empty(
            (0,),
            dtype=np.int64,
        )
        validation_signals = dataset.get(
            "val_signals",
            empty_signals,
        )
        validation_labels = dataset.get(
            "val_labels",
            empty_labels,
        )
        test_signals = dataset.get(
            "test_stream_signals",
            empty_signals,
        )
        test_labels = dataset.get(
            "test_stream_labels",
            empty_labels,
        )
        threshold = _compute_threshold(
            model,
            validation_signals,
            validation_labels,
        )
        reconstruction_scores = _compute_scores(
            model,
            test_signals,
        )
        new_thresholds[node] = round(
            threshold,
            4,
        )
        new_scores[node] = (
            reconstruction_scores
        )
        new_labels[node] = test_labels

        print(
            f"    {node}: "
            f"n={len(reconstruction_scores)}, "
            f"threshold={threshold:.4f}"
        )

    if not new_scores:
        # No node provided scores. Do not clear the existing panels; leave them,
        # and do not bump scores_round either, so the next tick retries.
        #
        # But the fact that the model was read must be recorded here. Otherwise
        # has_new_model stays true, and every tick while scores are empty
        # downloads the 213 KB global model from the CSE, unpacks gzip+base64 and
        # loads it with torch. That CPU load keeps Flask from serving requests and
        # the dashboard hangs at "Connecting...".
        if model is not None:
            with _shared_lock:
                _shared["model_round"] = model_round

        # While there are no uploads from this run yet (round 0, training round 1), record what was seen
        # so it is not re-read every tick. When a node uploads, uploads changes
        # and it is read again then.
        with _shared_lock:
            _shared["scores_round"] = current_fl_round
            _shared["scores_uploads"] = uploads

        print(
            "  ⚠ [Scores] 읽을 점수가 없다 "
            "(PKL 없음 + 드롭박스 비어 있음). 기존 값 유지."
        )
        return

    with _shared_lock:
        # Key fix:
        # Keep the existing stream position even when the global model changes.
        current_score_idx = _shared[
            "score_idx"
        ]

        _shared["thresholds"] = (
            new_thresholds
        )

        _shared["scores"] = new_scores
        _shared["labels"] = new_labels

        _shared["score_idx"] = (
            current_score_idx
        )

        if model is not None:
            _shared["model_round"] = (
                model_round
            )

        _shared["scores_round"] = current_fl_round
        _shared["scores_uploads"] = uploads

        _shared["summary_sent"] = False

    _broadcast(
        {
            "type": "threshold",
            "model_round": model_round,
            "thresholds": new_thresholds,
        }
    )

    print(
        f"  ✓ R{model_round} score 재계산 완료"
    )


# ════════════════════════════════════════════════════════
# Final summary computation
# ════════════════════════════════════════════════════════

def _compute_summary() -> dict | None:
    with _shared_lock:
        scores = {
            key: value.copy()
            for key, value
            in _shared["scores"].items()
        }
        labels = {
            key: value.copy()
            for key, value
            in _shared["labels"].items()
        }
        thresholds = dict(
            _shared["thresholds"],
        )
        model_round = _shared[
            "model_round"
        ]
        max_rounds = _shared[
            "max_rounds"
        ]
        evals = list(
            _shared.get("evals", [])
        )

    if (
        model_round < max_rounds
        or not scores
    ):
        return None

    # The live stream has no labels. So figures like AUROC and false alarms
    # come from the fixed validation set (valset) evaluation results. The one with the highest round is
    # used; if none, it is left as None and the corresponding block is hidden on screen.
    global_eval = None
    if evals:
        def _round_of(e: dict) -> int:
            try:
                return int(e.get("round", -1))
            except (TypeError, ValueError):
                return -1

        global_eval = max(evals, key=_round_of)

    result = {
        "type": "summary",
        "model_round": model_round,
        "global_eval": global_eval,
        "nodes": {},
    }

    for node in NODES:
        node_scores = scores.get(
            node,
        )
        node_labels = labels.get(
            node,
        )
        threshold = thresholds.get(
            node,
            1.0,
        )

        if (
            node_scores is None
            or node_labels is None
            or len(node_scores) == 0
        ):
            continue

        try:
            if len(
                np.unique(
                    node_labels,
                )
            ) > 1:
                auroc = float(
                    roc_auc_score(
                        node_labels,
                        node_scores,
                    )
                )
            else:
                auroc = 0.0
        except Exception:
            auroc = 0.0

        onset_index = next(
            (
                int(index)
                for index, label
                in enumerate(node_labels)
                if label == 1
            ),
            None,
        )

        detection_index = None
        consecutive = 0

        for index, score in enumerate(
            node_scores,
        ):
            if score > threshold:
                consecutive += 1

                if consecutive >= K_CONSECUTIVE:
                    detection_index = int(
                        index
                        - K_CONSECUTIVE
                        + 1
                    )
                    break
            else:
                consecutive = 0

        if (
            detection_index is not None
            and onset_index is not None
        ):
            detection_delay = int(
                detection_index
                - onset_index
            )
        else:
            detection_delay = None

        false_alarms = 0
        false_alarm_consecutive = 0

        for score, label in zip(
            node_scores,
            node_labels,
        ):
            if label != 0:
                continue

            if score > threshold:
                false_alarm_consecutive += 1
                if (
                    false_alarm_consecutive
                    >= K_CONSECUTIVE
                ):
                    false_alarms += 1
                    false_alarm_consecutive = 0
            else:
                false_alarm_consecutive = 0

        # Number of points where K consecutive exceedances 'start'. Same rule as verdict.py,
        # so it matches the number of verdicts actually uploaded to the CSE.
        detections = 0
        run = 0
        for score in node_scores:
            if score > threshold:
                run += 1
                if run == K_CONSECUTIVE:
                    detections += 1
            else:
                run = 0

        labeled = bool(
            len(
                np.unique(
                    node_labels,
                )
            ) > 1
        )

        entry = {
            "threshold": round(
                float(threshold),
                4,
            ),
            "labeled": labeled,

            # Things that can be said without labels
            "n_windows": int(
                len(node_scores)
            ),
            "exceed": int(
                np.sum(
                    node_scores > threshold
                )
            ),
            "detections": detections,
            "mean_score": round(
                float(
                    np.mean(node_scores)
                ),
                4,
            ),
            "max_score": round(
                float(
                    np.max(node_scores)
                ),
                4,
            ),
        }

        # Things that only make sense with labels. The live stream has all-normal
        # labels, so AUROC printed as 0, and showing that 0 on screen would be
        # a lie. It is not included at all.
        if labeled:
            entry.update({
                "auroc": round(
                    auroc,
                    4,
                ),
                "onset_idx": onset_index,
                "detect_idx": detection_index,
                "delay": detection_delay,
                "false_alarms": false_alarms,
                "n_normal": int(
                    np.sum(
                        node_labels == 0
                    )
                ),
                "n_anomaly": int(
                    np.sum(
                        node_labels == 1
                    )
                ),
            })

        result["nodes"][node] = entry

    return result


# ════════════════════════════════════════════════════════
# Pick test stream indices to show
# ════════════════════════════════════════════════════════

def _select_stream_index(
    node: str,
    node_labels: np.ndarray,
    stream_index: int,
    fl_round: int,
) -> int | None:
    """
    Pick the sample indices to show based on the node and current FL round.
    Default behavior:
      MN1:
        repeat normal samples
      MN2:
        repeat normal samples
      MN3 Round 1~6:
        repeat normal samples
      MN3 Round 7 or later:
        repeat anomalous samples
    """

    if len(node_labels) == 0:
        return None

    if node == ANOMALY_NODE:
        if fl_round >= ANOMALY_START_ROUND:
            target_label = 1
        else:
            target_label = 0
    else:
        target_label = 0

    candidate_indices = np.flatnonzero(
        node_labels == target_label
    )

    # If there is no such label, use all data
    if len(candidate_indices) == 0:
        candidate_indices = np.arange(
            len(node_labels),
        )
    if len(candidate_indices) == 0:
        return None

    selected_position = (
        stream_index
        % len(candidate_indices)
    )

    return int(
        candidate_indices[
            selected_position
        ]
    )


# ════════════════════════════════════════════════════════
# Score streaming thread
# ════════════════════════════════════════════════════════

def _score_thread() -> None:
    """
    Periodically send test reconstruction scores.
    score_idx is not reset even when the global model changes.
    """

    while True:
        time.sleep(
            SCORE_INTERVAL,
        )

        with _shared_lock:
            scores = _shared["scores"]
            labels = _shared["labels"]
            thresholds = _shared["thresholds"]
            stream_index = _shared["score_idx"]
            fl_state = _shared["fl_state"]
            fl_round = _shared["round"]

        if (
            not scores
            or fl_state == "FL_COMPLETED"
        ):
            continue

        event: dict = {
            "type": "score",
            "round": fl_round,
            "anomaly_active": (
                fl_round
                >= ANOMALY_START_ROUND
            ),
        }

        for node in NODES:
            node_scores = scores.get(
                node,
            )
            node_labels = labels.get(
                node,
            )

            if (
                node_scores is None
                or node_labels is None
                or len(node_scores) == 0
                or len(node_labels) == 0
            ):
                continue

            source_index = _select_stream_index(
                node=node,
                node_labels=node_labels,
                stream_index=stream_index,
                fl_round=fl_round,
            )

            if source_index is None:
                continue

            if source_index >= len(node_scores):
                continue

            event[node] = round(
                float(
                    node_scores[source_index]
                ),
                5,
            )
            event[f"{node}_label"] = int(
                node_labels[source_index]
            )
            event[f"{node}_thr"] = (
                thresholds.get(
                    node,
                    1.0,
                )
            )

        # Besides type, round and anomaly_active,
        # send only if at least one node's actual data is included
        if len(event) > 3:
            with _shared_lock:
                _shared["score_idx"] = (
                    stream_index + 1
                )
            _broadcast(
                event,
            )


# ════════════════════════════════════════════════════════
# oneM2M CIN parsing
# ════════════════════════════════════════════════════════

def _parse_content_instance(
    content_instance: dict | None,
) -> dict | None:
    if (
        not content_instance
        or "m2m:cin" not in content_instance
    ):
        return None

    content = (
        content_instance["m2m:cin"]
        .get("con")
    )

    try:
        if isinstance(
            content,
            str,
        ):
            return json.loads(
                content,
            )
        if isinstance(
            content,
            dict,
        ):
            return content

    except (
        TypeError,
        ValueError,
        json.JSONDecodeError,
    ):
        return None

    return None


# ════════════════════════════════════════════════════════
# FL state polling thread
# ════════════════════════════════════════════════════════

DASHBOARD_STARTED_AT = time.time()
IGNORE_STALE_CONTROL = os.getenv("FL_DASH_IGNORE_STALE", "1") != "0"
_stale_control_warned = False


def _is_stale_payload(content: dict) -> bool:
    """
    Check whether a CIN is left over from a previous run. Same criterion as _is_stale_control
    but without printing a warning. Used where it must be filtered quietly, like evaluation results.
    """
    if not IGNORE_STALE_CONTROL:
        return False

    try:
        ts = float(content.get("timestamp", 0))
    except (TypeError, ValueError):
        return False

    return 0 < ts < DASHBOARD_STARTED_AT


def _is_stale_control(content: dict) -> bool:
    """
    Check whether a round command is left over from a previous run.

    cnt-fl-control only gets CINs appended each round and is never cleared.
    So even after a fresh start, until the IN-AE posts its first command the previous run's
    last state is shown as is. That is the "dashboard starts from round 2" issue.
    In run_fl.sh the dashboard is [2/5] and the IN-AE is [5/5], so
    the gap is tens of seconds.

    Uses the same criterion the MN-AE uses to filter stale commands. If the payload's
    timestamp is earlier than this dashboard's start time, it belongs to a previous run.
    Turning it off with FL_DASH_IGNORE_STALE=0 shows it as is, as before.
    """
    global _stale_control_warned

    if not IGNORE_STALE_CONTROL:
        return False

    try:
        ts = float(content.get("timestamp", 0))
    except (TypeError, ValueError):
        return False

    if ts <= 0 or ts >= DASHBOARD_STARTED_AT:
        return False

    if not _stale_control_warned:
        _stale_control_warned = True
        print(
            f"  ⓘ 지난 실행의 라운드 상태를 무시한다 "
            f"(R{content.get('currentRound')} "
            f"{content.get('jobState')}). "
            f"IN-AE 가 새 명령을 올릴 때까지 대기한다."
        )

    return True


def _control_thread() -> None:
    """
    Watch only the round state. Push to the screen as soon as it changes.

    jobState / currentRound / maxRounds are modified only by this thread. If _poll_thread
    modified them, it could overwrite with an old round read at the start of its slow cycle, and the screen
    would go back a round and then return.
    """
    fl_control_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-fl-control"
    )
    last = None

    while True:
        try:
            content = _parse_content_instance(
                om2m.get_latest_content_instance(fl_control_path)
            )
            if content and not _is_stale_control(content):
                state = (
                    content.get("jobState", "FL_READY"),
                    int(content.get("currentRound", 0)),
                    int(content.get("maxRounds", config.GLOBAL_ROUNDS)),
                )
                if state != last:
                    last = state
                    with _shared_lock:
                        _shared.update(
                            {
                                "fl_state": state[0],
                                "round": state[1],
                                "max_rounds": state[2],
                            }
                        )
                        nodes = dict(_shared["nodes"])
                    _broadcast(
                        {
                            "type": "round",
                            "fl_state": state[0],
                            "round": state[1],
                            "max_rounds": state[2],
                            "nodes": nodes,
                        }
                    )
                    print(f"  [Round] R{state[1]}/{state[2]} {state[0]}")
        except Exception as error:
            print(f"  ⚠ 라운드 상태 조회 오류: {error}")

        time.sleep(CONTROL_POLL_INTERVAL)


def _poll_thread() -> None:
    # The IN-AE's containers are on the central IN-CSE. config.CSE_NAME is the site
    # MN-CSE name, so building paths with it looks for resources that do not exist.
    fl_control_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-fl-control"
    )
    local_updates_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-local-updates"
    )
    # Where the IN-AE posts results measured on the fixed validation set each round.
    fl_inference_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-fl-inference"
    )

    previous_snapshot: dict = {}
    last_eval_round = -1

    while True:
        try:
            control_content = (
                _parse_content_instance(
                    om2m.get_latest_content_instance(
                        fl_control_path,
                    )
                )
            )

            if control_content and _is_stale_control(control_content):
                # Afterimage of a previous run. Change nothing until a new command arrives.
                control_content = None

            if control_content:
                fl_state = control_content.get(
                    "jobState",
                    "FL_READY",
                )
                round_number = int(
                    control_content.get(
                        "currentRound",
                        0,
                    )
                )
                max_rounds = int(
                    control_content.get(
                        "maxRounds",
                        config.GLOBAL_ROUNDS,
                    )
                )

                nodes: dict = {}

                for node in NODES:
                    node_content = (
                        _parse_content_instance(
                            om2m.get_latest_content_instance(
                                (
                                    f"{local_updates_path}/"
                                    f"cnt-{node}"
                                )
                            )
                        )
                    )

                    if not node_content:
                        continue

                    # Uploads left by a previous run (before this dashboard started) do not belong to this run.
                    if _is_stale_payload(node_content):
                        continue

                    nodes[node] = {
                        "train_loss": round(
                            float(
                                node_content.get(
                                    "train_loss",
                                    0,
                                )
                            ),
                            5,
                        ),
                        "val_loss": round(
                            float(
                                node_content.get(
                                    "val_loss",
                                    0,
                                )
                            ),
                            5,
                        ),
                        "val_auroc": round(
                            float(
                                node_content.get(
                                    "val_auroc",
                                    0,
                                )
                            ),
                            4,
                        ),
                        "num_samples": int(
                            node_content.get(
                                "num_samples",
                                0,
                            )
                        ),
                        "round": int(
                            node_content.get(
                                "round",
                                0,
                            )
                        ),
                        # The MN-CSE the site app is currently attached to and the handover count.
                        # If greater than 0, show the spare indicator on screen.
                        "mn_cse": node_content.get("mn_cse"),
                        "mn_base": node_content.get("mn_base"),
                        "cse_handovers": int(
                            node_content.get(
                                "cse_handovers",
                                0,
                            ) or 0
                        ),
                    }

                # Round state is handled by _control_thread. Here it is sent only when
                # node uploads change.
                snapshot = {
                    "node_keys": sorted(
                        nodes.keys(),
                    ),
                    "rounds": {
                        node: data["round"]
                        for node, data
                        in nodes.items()
                    },

                    "losses": {
                        node: data[
                            "train_loss"
                        ]
                        for node, data
                        in nodes.items()
                    },
                }

                if snapshot != previous_snapshot:
                    previous_snapshot = snapshot

                    with _shared_lock:
                        _shared["nodes"] = nodes
                        cur_state = _shared["fl_state"]
                        cur_round = _shared["round"]
                        cur_max = _shared["max_rounds"]

                    _broadcast(
                        {
                            "type": "round",
                            "fl_state": cur_state,
                            "round": cur_round,
                            "max_rounds": cur_max,
                            "nodes": nodes,
                        }
                    )
                    print(
                        "  [Poll] "
                        f"R{cur_round}/{cur_max} "
                        f"{cur_state} "
                        f"nodes={ {n: d['round'] for n, d in nodes.items()} }"
                    )

            # Per-round evaluation results. Emit each new round's once.
            ev = _parse_content_instance(
                om2m.get_latest_content_instance(fl_inference_path)
            )
            if (ev and ev.get("type") == "global-eval"
                    and int(ev.get("round", -1)) > last_eval_round
                    and not _is_stale_payload(ev)):
                last_eval_round = int(ev["round"])
                ev = dict(ev)
                ev["type"] = "eval"
                # Send to those connected, and keep them so those connecting later can also
                # see past rounds. This fixes past rounds disappearing entirely
                # on refresh.
                with _shared_lock:
                    _shared["evals"].append(ev)
                    del _shared["evals"][:-EVAL_HISTORY_MAX]
                _broadcast(ev)
                print(f"  [Eval] R{last_eval_round} "
                      f"AUROC={ev.get('auroc')} "
                      f"탐지={ev.get('detected')}/{ev.get('n_anomaly')} "
                      f"오경보={ev.get('false_alarms')}/{ev.get('n_normal')}")

        except Exception as error:
            print(
                "  ⚠ oneM2M poll 오류: "
                f"{error}"
            )

        # Check whether there is a new global model
        _reload_scores_if_needed()

        # Once FL is complete and the final model is ready,
        # send the summary only once.
        with _shared_lock:
            fl_completed = (
                _shared["fl_state"]
                == "FL_COMPLETED"
            )
            summary_already_sent = (
                _shared["summary_sent"]
            )
            final_model_ready = (
                _shared["model_round"]
                >= _shared["max_rounds"]
            )

        if (
            fl_completed
            and final_model_ready
            and not summary_already_sent
        ):
            summary = _compute_summary()

            if summary:
                with _shared_lock:
                    _shared["summary_sent"] = True

                _broadcast(
                    summary,
                )
                print(
                    "  ✓ Summary 전송 완료 "
                    f"(R{summary['model_round']})"
                )
        time.sleep(
            POLL_INTERVAL,
        )


# ════════════════════════════════════════════════════════
# Main
# ════════════════════════════════════════════════════════

def main() -> None:
    # The dashboard looks at both sides.
    #   Site MN-CSE     per-window anomaly scores (read site by site)
    #   Central IN-CSE  round state / per-site summary / shared model / per-round evaluation
    config.require_sandbox()
    om2m.set_endpoint(*config.mn_endpoint("mn1"))
    om2m.set_in_endpoint(*config.in_endpoint())

    print()
    print("=== FL Dashboard Server ===")
    print("  사업장 CSE: " + ", ".join(
        f"{n} {config.mn_endpoint(n)[1]}" for n in NODES))
    print(f"  중앙 CSE  : {config.in_endpoint()[0]}/{om2m.in_cse_name()}")
    print(f"  PKL: {PKL_DIR}")
    print(
        "  이상 표시: "
        f"{ANOMALY_NODE}, "
        f"Round {ANOMALY_START_ROUND}부터"
    )
    print(f"  Port: {PORT}")
    print(f"  http://localhost:{PORT}")
    print()

    polling_thread = threading.Thread(
        target=_poll_thread,
        daemon=True,
        name="fl-poll",
    )

    control_thread = threading.Thread(
        target=_control_thread,
        daemon=True,
        name="fl-control",
    )

    score_thread = threading.Thread(
        target=_score_thread,
        daemon=True,
        name="score-stream",
    )

    polling_thread.start()
    control_thread.start()
    score_thread.start()

    app.run(
        host="0.0.0.0",
        port=PORT,
        threaded=True,
        use_reloader=False,
    )


if __name__ == "__main__":
    main()