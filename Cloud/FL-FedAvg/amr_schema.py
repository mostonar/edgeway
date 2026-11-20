"""
amr_schema.py — channel spec and condition vector definition for AMR driving data

Gathered in one place so prepare_data_amr (offline files) and amr_ingest (live CSE) use
the same definitions. To change channels or the condition layout, edit only this file.

[38 channels]
    IMU   front/left/right × (ax ay az gx gy gz roll pitch) = 24
    ENC   drive(pos,vel,eff) swivel(pos) roller_fl/fr/rl/rr(pos,vel) lift(pos,eff) = 14

[8-dim condition vector]
    nav group 5 : stop / straight / turn / load-unload / other
    speed bucket 3 : low / mid / high   (mean of |drive.vel|)

In operation there are no labels like cond:normal. The robots just drive and
upload data. So the condition is derived from the payload. nav and speed are
available as-is at inference time too.

over_bump is not part of the condition. Including it would amount to telling the model
"this is a bump", which defeats anomaly detection. It is grouped with straight.
"""

from __future__ import annotations

import os
from typing import Iterable, List, Optional, Tuple

import numpy as np

# ── Channels ──────────────────────────────────────────────────
CH_SPEC = [("imu", p, c) for p in ("front", "left", "right")
           for c in ("ax", "ay", "az", "gx", "gy", "gz", "roll", "pitch")]
CH_SPEC += [("enc", "drive", c) for c in ("pos", "vel", "eff")]
CH_SPEC += [("enc", "swivel", "pos")]
CH_SPEC += [("enc", f"roller_{s}", c)
            for s in ("fl", "fr", "rl", "rr") for c in ("pos", "vel")]
CH_SPEC += [("enc", "lift", c) for c in ("pos", "eff")]

CH_NAMES = [f"{p}.{c}" for _, p, c in CH_SPEC]
IS_POS = np.array([c == "pos" for _, _, c in CH_SPEC])
N_CH = len(CH_SPEC)
VEL_IDX = CH_NAMES.index("drive.vel")
LIFT_EFF_IDX = CH_NAMES.index("lift.eff")

T = int(os.getenv("AMR_WINDOW", "60"))

# ── Condition ─────────────────────────────────────────────────
NAV_GROUPS = ["stop", "straight", "turn", "loadunload", "other"]
N_NAV = len(NAV_GROUPS)
N_SPD = 3

# Load state. Isaac Sim does not send weight, so it is read from the nav 'segment'.
#   to_pick*  heading to pick up     -> empty
#   to_drop*  heading to drop off    -> loaded
#   loading / unloading        -> loading or unloading
# With a load, inertia grows, so IMU acceleration is damped and drive.eff rises.
# If that difference is not given as a condition, everything is read as 'anomaly'.
LOAD_STATES = ["empty", "light", "heavy"]
N_LOAD = len(LOAD_STATES)

# lift.eff boundaries. Measured histogram (Isaac Sim, 1857 windows, bins of 500):
#   4500~5000  1166   <- empty. Tightly clustered around 4561
#   5000~5500    79
#   5500~6000    73
#   6000~6500    80
#   6500~7000    79
#   7000~7500   136
#   7500~8000   137
#   8000~8500    98
# Drops off sharply at 5000 (1166 -> 79). The empty/loaded boundary is clean here.
# Above that it spreads evenly over 5000~8500. The load weight varies continuously,
# so instead of one 'loaded' bucket it is split into light/heavy.
LIFT_EDGES = tuple(
    float(x) for x in os.getenv("AMR_LIFT_EDGES", "5000,6500").split(",")
)

COND_DIM = N_NAV + N_SPD + N_LOAD

# nav string -> group. Unknown values map to 'other'.
NAV_TO_GROUP = {
    "idle": 0, "done": 0, "reset": 0, "stopped": 0,
    "to_pick": 1, "to_drop": 1, "to_aisle": 1, "to_lane": 1,
    "straight": 1,
    # Crossing a bump counts as 'straight'. A separate group would leak the answer.
    "over_bump": 1,
    "turn_in": 2, "turn_out": 2, "turn_drop": 2, "turn_unload": 2,
    "turn_start": 2, "turn_lane": 2, "left": 2, "right": 2,
    "loading": 3, "unloading": 3,
    "aborted": 4,
}

# Speed bucket boundaries. Training and inference must use the same values.
SPD_EDGES = tuple(
    float(x) for x in os.getenv("AMR_SPD_EDGES", "2.0,6.0").split(",")
)

# Below this speed the window is treated as 'stopped' and dropped.
# Since this uses actual speed rather than the nav label, brief stops while driving survive.
MIN_SPEED = float(os.getenv("AMR_MIN_SPEED", "0.1"))

# nav values excluded from training. Aborted runs are hard to treat as normal driving.
EXCLUDE_NAV_TRAIN = {
    n.strip() for n in os.getenv("AMR_EXCLUDE_NAV", "aborted").split(",")
    if n.strip()
}


def nav_parts(nav: Optional[str]) -> list:
    """'to_drop/turn_unload' -> ['to_drop', 'turn_unload']"""
    return [p for p in str(nav or "").strip().split("/") if p]


def nav_group(nav: Optional[str]) -> int:
    """
    nav string -> motion group.

    Isaac Sim sends segment and motion joined by '/', like 'to_pick/turn_side'.
    Previously the whole string was looked up in a dict, and any miss fell into 'other'.
    In measurements, 926 of 1857 (49.9%) piled into the single 'other' bucket that way,
    and nothing went into the turn group. Now the pieces are looked up separately.

    The trailing piece (motion) takes priority over the leading piece (segment). 'to_drop/turn_out'
    is a turn, not straight.
    """
    raw = str(nav or "").strip()
    if raw in NAV_TO_GROUP:
        return NAV_TO_GROUP[raw]
    for part in reversed(nav_parts(raw)):
        if part in NAV_TO_GROUP:
            return NAV_TO_GROUP[part]
    return 4


def load_state_from_nav(nav: Optional[str]) -> int:
    """
    Pick the load state from the nav string alone. Fallback when lift.eff is missing.

    nav says 'which segment is being driven', not 'what is being carried
    right now'. In measurements, 309 of 828 to_drop windows (37%) had lift.eff at
    empty level, and 3 robots drove the to_drop segment entirely empty.
    So this is strictly a fallback.
    """
    parts = nav_parts(nav)
    if any(p in ("loading", "unloading") for p in parts):
        return 1
    for p in parts:
        if p.startswith("to_drop"):
            return 2
    return 0


def load_state(nav: Optional[str] = None,
               window: Optional[np.ndarray] = None) -> int:
    """
    Load state index. Uses lift.eff (measured) first, falls back to nav if missing.

    Why the measurement first? nav is a label Isaac Sim attaches, so if names change it is
    silently wrong. In fact, composite labels like 'to_pick/turn_side' had put half of
    everything into the single 'other' bucket and nobody noticed. lift.eff is the force needed
    to lift the load, so it tracks weight regardless of labels.
    """
    if window is not None:
        try:
            eff = float(np.nanmean(window[LIFT_EFF_IDX]))
        except (IndexError, ValueError, TypeError):
            eff = float("nan")
        if np.isfinite(eff):
            if eff < LIFT_EDGES[0]:
                return 0        # empty
            if eff < LIFT_EDGES[1]:
                return 1        # light
            return 2            # heavy
    return load_state_from_nav(nav)


def speed_of(window: np.ndarray) -> float:
    """Mean driving speed of one window (|drive.vel|)."""
    return float(np.abs(window[VEL_IDX]).mean())


def speed_bucket(speed: float) -> int:
    if speed < SPD_EDGES[0]:
        return 0
    if speed < SPD_EDGES[1]:
        return 1
    return 2


def cond_vector(nav: Optional[str], speed: float,
                window: Optional[np.ndarray] = None) -> np.ndarray:
    """
    Condition vector. If window is given, the load state is determined from lift.eff.

    window must be the raw (C, T) signal returned by window_from_con().
    Subtracting the mean of the .pos channels does not affect lift.eff.
    """
    v = np.zeros(COND_DIM, dtype=np.float32)
    v[nav_group(nav)] = 1.0
    v[N_NAV + speed_bucket(speed)] = 1.0
    v[N_NAV + N_SPD + load_state(nav, window)] = 1.0
    return v


# ── Legacy label conversion ───────────────────────────────────
# Old data (merged.json) has scn/spd/load labels.
# Mapped into the same 8 dims to match the new layout. load does not exist in the new data, so it is dropped.
LEGACY_SCN_TO_NAV = {
    "idle": "idle", "straight": "straight", "left": "left", "right": "right",
}
LEGACY_SPD_TO_SPEED = {"none": 0.0, "low": 1.0, "mid": 4.0, "high": 8.0}


def cond_from_labels(scn: str, spd: str) -> np.ndarray:
    return cond_vector(
        LEGACY_SCN_TO_NAV.get(scn, scn),
        LEGACY_SPD_TO_SPEED.get(spd, 0.0),
    )


# ── Window extraction ─────────────────────────────────────────
def window_from_con(payload: dict) -> Optional[np.ndarray]:
    """
    One CIN con (dict) -> signal (C, T). None if channels are missing.
    For cumulative positions (.pos), subtract the window mean so only 'the change within this window' remains.
    """
    cols = []
    for grp, part, chan in CH_SPEC:
        seq = payload.get(grp, {}).get(part, {}).get(chan)
        if not isinstance(seq, list) or len(seq) < T:
            return None
        cols.append([0.0 if v is None else float(v) for v in seq[:T]])

    sig = np.asarray(cols, dtype=np.float32)
    sig[IS_POS, :] -= sig[IS_POS, :].mean(axis=1, keepdims=True)
    return sig


def pose_of(payload: dict) -> Tuple[float, float]:
    px = [v for v in (payload.get("pose", {}).get("x") or []) if v is not None]
    py = [v for v in (payload.get("pose", {}).get("y") or []) if v is not None]
    return (float(np.mean(px)) if px else float("nan"),
            float(np.mean(py)) if py else float("nan"))


def moving_mask(X: np.ndarray) -> np.ndarray:
    """Filter out stopped windows using actual speed."""
    if len(X) == 0:
        return np.zeros(0, dtype=bool)
    return np.array([speed_of(w) >= MIN_SPEED for w in X])


# ════════════════════════════════════════════════════════
# Shared routine that z-scores per-channel values against the condition-cell reference
#
# The training side (edge_node) and the evaluation side (valset) must use the same ruler, or
# per-round numbers and operational verdicts disagree. So it lives in one place here.
#
# [Why an MAD floor is needed]
# Some channels are constant for the whole capture (e.g. lift.pos when driving unloaded).
# Their MAD is 0, and dividing by something near 0 makes z jump as high as 10^7.
# That is exactly why every site once looked equally "anomalous".
# Two guards:
#   - Channels that are constant overall are dropped entirely (z=0). They carry no information.
#   - A cell's MAD cannot fall below a fixed fraction of that channel's overall MAD.
#     This prevents blow-ups when a cell has few samples that happen to be clustered.
# ════════════════════════════════════════════════════════

MAD_K = 1.4826          # Scale MAD to the same units as the standard deviation under a normal distribution
MAD_FLOOR_REL = 0.1     # Cell MAD floor = 10% of the channel's overall MAD


def _mad(A, axis=0):
    med = np.median(A, axis=axis)
    return med, np.median(np.abs(A - med), axis=axis) * MAD_K


def robust_ref(D, keys, min_cell: int = 8):
    """Per-channel values (N, C) -> (per-cell reference, overall reference, live-channel mask).

    D    : (N, C) one value per window per channel
    keys : (N,)   condition cell index
    Returns : ({cell: (median, MAD)}, (median, MAD), valid-channel bool (C,))
    """
    D = np.asarray(D, dtype=np.float64)
    g_med, g_mad = _mad(D)
    alive = g_mad > 0                      # Drop channels that are constant overall
    floor = np.where(alive, g_mad * MAD_FLOOR_REL, np.inf)
    g_mad = np.maximum(g_mad, floor)

    ref = {}
    keys = np.asarray(keys)
    for k in np.unique(keys):
        m = keys == k
        if m.sum() < min_cell:
            continue
        md, mad = _mad(D[m])
        ref[int(k)] = (md, np.maximum(mad, floor))
    return ref, (g_med, g_mad), alive


def apply_ref(D, keys, ref, glob, alive):
    """z-score against the robust_ref reference (N, C). Dead channels are set to 0."""
    D = np.asarray(D, dtype=np.float64)
    g_med, g_mad = glob
    keys = (np.asarray(keys) if keys is not None
            else np.zeros(len(D), dtype=np.int64))
    Z = np.empty_like(D)
    for i, k in enumerate(keys):
        md, mad = ref.get(int(k), (g_med, g_mad))
        Z[i] = (D[i] - md) / mad
    Z[:, ~alive] = 0.0
    return Z.astype(np.float32)


# ════════════════════════════════════════════════════════
# Per-site input normalization statistics
#
# [Why not freeze at R1]
# Previously the mean/std were taken from the first batch (~50 windows) and used to the end.
# Isaac Sim starts with empty robots, so the first batch is almost all empty. Measured
# (run of 2026-11-19 01:19, siteA): R1 was 45/45 empty, R6 was 0/50 empty.
# R1's lift.pos std was 6.7e-5, so once loaded segments arrived |z| jumped to 40~80,
# and the training loss rising from 1.1 -> 14.3 was not training breaking down
# but this ruler being off (the reproduced mean z² nearly matches the loss).
#
# [How it works]
#   - Until each load state (empty/light/heavy) has at least NORM_MIN_PER_LOAD windows,
#     keep accumulating received windows and recompute the statistics.
#   - Once all are collected or NORM_FREEZE_ROUND is reached, freeze. If the ruler changed
#     after that, scores could not be compared across rounds.
#   - A std floor and |z| clipping are applied together. Even before freezing, this is a
#     safeguard so a single rare window cannot dominate the loss.
#
# Per-round mean z² reproduced from the same telemetry (siteA):
#   Frozen at R1     1.0 2.9 1.4 5.4 13.6 16.7 16.8 15.4 10.8 5.1
#   Accumulate then freeze 1.0 1.2 0.6 0.6  0.7  0.6  0.5  2.0  0.8 0.6  (frozen at R5)
# ════════════════════════════════════════════════════════

NORM_MIN_PER_LOAD = int(os.getenv("AMR_NORM_MIN_PER_LOAD", "20"))
NORM_FREEZE_ROUND = int(os.getenv("AMR_NORM_FREEZE_ROUND", "5"))
NORM_STD_FLOOR = float(os.getenv("AMR_NORM_STD_FLOOR", "1e-4"))
NORM_CLIP = float(os.getenv("AMR_NORM_CLIP", "10"))
# Cap on raw-signal windows to keep. Used only until freezing, then discarded.
NORM_MAX_BUFFER = int(os.getenv("AMR_NORM_MAX_BUFFER", "2000"))


class NormStats:
    """Per-channel normalization statistics for one site. Accumulated until frozen."""

    def __init__(self,
                 min_per_load: int = NORM_MIN_PER_LOAD,
                 freeze_round: int = NORM_FREEZE_ROUND,
                 std_floor: float = NORM_STD_FLOOR,
                 clip: float = NORM_CLIP,
                 max_buffer: int = NORM_MAX_BUFFER):
        self.min_per_load = int(min_per_load)
        self.freeze_round = int(freeze_round)
        self.std_floor = float(std_floor)
        self.clip = float(clip)
        self.max_buffer = int(max_buffer)

        self.mean: Optional[np.ndarray] = None   # (1, C, 1)
        self.std: Optional[np.ndarray] = None    # (1, C, 1)
        self.frozen = False
        self.frozen_round: Optional[int] = None
        self._buf: List[np.ndarray] = []
        self._load_counts = np.zeros(N_LOAD, dtype=np.int64)

    @staticmethod
    def load_of(cond: np.ndarray) -> np.ndarray:
        """cond vector (N, COND_DIM) -> load state index (N,)."""
        C = np.asarray(cond)
        lo = N_NAV + N_SPD
        if C.ndim != 2 or C.shape[1] < lo + N_LOAD:
            return np.zeros(len(C), dtype=np.int64)
        return np.argmax(C[:, lo:lo + N_LOAD], axis=1).astype(np.int64)

    def update(self, X: np.ndarray, cond: Optional[np.ndarray],
               round_num: int) -> None:
        """Update the statistics with this round's raw-signal windows (N, C, T). Ignored once frozen."""
        if self.frozen or X is None or len(X) == 0:
            return
        X = np.asarray(X, dtype=np.float32)
        self._buf.append(X)
        if cond is not None and len(cond) == len(X):
            self._load_counts += np.bincount(self.load_of(cond),
                                             minlength=N_LOAD)[:N_LOAD]

        A = np.concatenate(self._buf)
        if len(A) > self.max_buffer:
            # If over the cap, keep an even spread across the whole time axis.
            keep = np.linspace(0, len(A) - 1, self.max_buffer).astype(np.int64)
            A = A[keep]
            self._buf = [A]

        self.mean = A.mean(axis=(0, 2), keepdims=True)
        self.std = np.maximum(A.std(axis=(0, 2), keepdims=True),
                              self.std_floor)

        covered = bool(self._load_counts.min() >= self.min_per_load)
        if covered or round_num >= self.freeze_round:
            self.frozen = True
            self.frozen_round = int(round_num)
            self._buf = []

    def apply(self, X: np.ndarray) -> np.ndarray:
        """(N, C, T) raw signal -> standardize + clip."""
        if X is None or np.asarray(X).size == 0:
            return X
        if self.mean is None:
            raise RuntimeError("NormStats.apply: update() 를 먼저 불러야 한다")
        Z = (np.asarray(X, dtype=np.float32) - self.mean) / self.std
        if self.clip > 0:
            Z = np.clip(Z, -self.clip, self.clip)
        return Z.astype(np.float32)

    def describe(self) -> str:
        cnt = "/".join(str(int(c)) for c in self._load_counts)
        if self.frozen:
            return (f"정규화 통계 고정됨 (R{self.frozen_round}, "
                    f"적재 빈/경/중 {cnt}, |z|≤{self.clip:g})")
        return (f"정규화 통계 누적 중 (적재 빈/경/중 {cnt}, "
                f"각 {self.min_per_load}창 또는 R{self.freeze_round} 에 고정)")
