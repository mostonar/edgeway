"""
amr_schema.py — AMR 주행 데이터의 채널 규격과 조건 벡터 정의

prepare_data_amr(오프라인 파일)과 amr_ingest(라이브 CSE)가 같은 정의를 쓰도록
한 곳에 모았다. 채널을 바꾸거나 조건 구성을 바꿀 때 여기만 고치면 된다.

[채널 38개]
    IMU   front/left/right × (ax ay az gx gy gz roll pitch) = 24
    ENC   drive(pos,vel,eff) swivel(pos) roller_fl/fr/rl/rr(pos,vel) lift(pos,eff) = 14

[조건 벡터 8차원]
    nav 그룹 5 : 정지 / 직진 / 회전 / 적재·하역 / 기타
    속도 버킷 3 : 저 / 중 / 고   (drive.vel 절대값 평균)

운영 중에는 cond:normal 같은 라벨이 붙지 않는다. 로봇은 그냥 주행하고
데이터를 올릴 뿐이다. 그래서 조건은 payload 안에서 끌어낸다. nav와 속도는
추론 시점에도 그대로 얻을 수 있다.

over_bump 는 조건에 넣지 않는다. 넣으면 "여기가 단차다"를 모델에 알려주는
셈이라 이상 탐지가 성립하지 않는다. 직진 그룹으로 묶는다.
"""

from __future__ import annotations

import os
from typing import Iterable, List, Optional, Tuple

import numpy as np

# ── 채널 ──────────────────────────────────────────────────
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

# ── 조건 ──────────────────────────────────────────────────
NAV_GROUPS = ["stop", "straight", "turn", "loadunload", "other"]
N_NAV = len(NAV_GROUPS)
N_SPD = 3

# 적재 상태. 아이작심이 무게를 안 보내므로 nav 의 '구간' 에서 읽는다.
#   to_pick*  픽업하러 가는 길  -> 빈 차
#   to_drop*  내려놓으러 가는 길 -> 적재
#   loading / unloading        -> 싣고 내리는 중
# 짐을 실으면 관성이 커져 IMU 가속도가 눌리고 drive.eff 가 더 든다.
# 그 차이를 조건으로 안 주면 전부 '이상' 으로 해석된다.
LOAD_STATES = ["empty", "light", "heavy"]
N_LOAD = len(LOAD_STATES)

# lift.eff 경계. 실측 히스토그램(아이작심 1857창, 500 단위):
#   4500~5000  1166   <- 빈차. 4561 에 아주 좁게 뭉쳐 있다
#   5000~5500    79
#   5500~6000    73
#   6000~6500    80
#   6500~7000    79
#   7000~7500   136
#   7500~8000   137
#   8000~8500    98
# 5000 에서 뚝 끊긴다(1166 -> 79). 빈차/적재 경계는 여기가 깨끗하다.
# 그 위는 5000~8500 에 고르게 퍼져 있다. 짐 무게가 연속으로 변한다는
# 뜻이라, 적재를 한 칸으로 묶지 않고 가벼움/무거움으로 나눈다.
LIFT_EDGES = tuple(
    float(x) for x in os.getenv("AMR_LIFT_EDGES", "5000,6500").split(",")
)

COND_DIM = N_NAV + N_SPD + N_LOAD

# nav 문자열 -> 그룹. 모르는 값은 'other'.
NAV_TO_GROUP = {
    "idle": 0, "done": 0, "reset": 0, "stopped": 0,
    "to_pick": 1, "to_drop": 1, "to_aisle": 1, "to_lane": 1,
    "straight": 1,
    # 단차 통과는 '직진'으로 둔다. 별도 그룹으로 만들면 정답 유출이다.
    "over_bump": 1,
    "turn_in": 2, "turn_out": 2, "turn_drop": 2, "turn_unload": 2,
    "turn_start": 2, "turn_lane": 2, "left": 2, "right": 2,
    "loading": 3, "unloading": 3,
    "aborted": 4,
}

# 속도 버킷 경계. 학습과 추론이 같은 값을 써야 한다.
SPD_EDGES = tuple(
    float(x) for x in os.getenv("AMR_SPD_EDGES", "2.0,6.0").split(",")
)

# 이 속도 아래는 '정지'로 보고 뺀다.
# nav 라벨이 아니라 실제 속도로 판정하므로, 주행 중 잠깐 멈춘 구간은 살아남는다.
MIN_SPEED = float(os.getenv("AMR_MIN_SPEED", "0.1"))

# 학습에서 뺄 nav. 중단된 주행은 정상 주행으로 보기 어렵다.
EXCLUDE_NAV_TRAIN = {
    n.strip() for n in os.getenv("AMR_EXCLUDE_NAV", "aborted").split(",")
    if n.strip()
}


def nav_parts(nav: Optional[str]) -> list:
    """'to_drop/turn_unload' -> ['to_drop', 'turn_unload']"""
    return [p for p in str(nav or "").strip().split("/") if p]


def nav_group(nav: Optional[str]) -> int:
    """
    nav 문자열 -> 동작 그룹.

    아이작심은 'to_pick/turn_side' 처럼 구간과 동작을 '/' 로 붙여 보낸다.
    예전에는 통째로 사전에서 찾아서 못 맞추면 전부 'other' 로 떨어졌다.
    실측에서 1857건 중 926건(49.9%)이 그렇게 'other' 한 칸에 몰렸고,
    회전 그룹에는 아무것도 안 들어갔다. 조각으로 나눠서 찾는다.

    뒤 조각(동작)을 앞 조각(구간)보다 우선한다. 'to_drop/turn_out' 은
    회전이지 직진이 아니다.
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
    nav 문자열만 보고 적재 상태를 고른다. lift.eff 가 없을 때의 보조.

    nav 는 '어느 구간을 달리는 중인가' 지 '지금 무엇을 들고 있는가' 가
    아니다. 실측에서 to_drop 828창 중 309창(37%)은 lift.eff 가 빈차
    수준이었고, 로봇 3대는 to_drop 구간을 아예 빈 차로 달렸다.
    그래서 이건 어디까지나 보조다.
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
    적재 상태 인덱스. lift.eff(측정값)를 1순위로 쓰고, 없으면 nav 로 간다.

    왜 측정값이 먼저인가. nav 는 아이작심이 붙이는 라벨이라 이름이 바뀌면
    조용히 틀린다. 실제로 'to_pick/turn_side' 같은 복합 라벨 때문에 전체의
    절반이 'other' 한 칸에 몰려 있었고 아무도 몰랐다. lift.eff 는 짐을
    들어올리는 데 드는 힘이라 라벨과 무관하게 무게를 따라간다.
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
    """윈도우 하나의 평균 주행 속도 (drive.vel 절대값)."""
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
    조건 벡터. window 를 주면 적재 상태를 lift.eff 로 정한다.

    window 는 window_from_con() 이 돌려준 (C, T) 원신호여야 한다.
    .pos 채널의 평균을 빼는 가공은 lift.eff 에 영향을 주지 않는다.
    """
    v = np.zeros(COND_DIM, dtype=np.float32)
    v[nav_group(nav)] = 1.0
    v[N_NAV + speed_bucket(speed)] = 1.0
    v[N_NAV + N_SPD + load_state(nav, window)] = 1.0
    return v


# ── 레거시 라벨 변환 ──────────────────────────────────────
# 옛 데이터(merged.json)는 scn/spd/load 라벨을 갖고 있다.
# 새 구성과 맞추기 위해 같은 8차원으로 옮긴다. load는 새 데이터에 없으므로 버린다.
LEGACY_SCN_TO_NAV = {
    "idle": "idle", "straight": "straight", "left": "left", "right": "right",
}
LEGACY_SPD_TO_SPEED = {"none": 0.0, "low": 1.0, "mid": 4.0, "high": 8.0}


def cond_from_labels(scn: str, spd: str) -> np.ndarray:
    return cond_vector(
        LEGACY_SCN_TO_NAV.get(scn, scn),
        LEGACY_SPD_TO_SPEED.get(spd, 0.0),
    )


# ── 윈도우 추출 ───────────────────────────────────────────
def window_from_con(payload: dict) -> Optional[np.ndarray]:
    """
    CIN의 con(dict) 하나 -> 신호 (C, T). 채널이 모자라면 None.
    누적 위치(.pos)는 윈도우 평균을 빼서 '이 구간의 변화'만 남긴다.
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
    """실제 속도로 정지 구간을 가려낸다."""
    if len(X) == 0:
        return np.zeros(0, dtype=bool)
    return np.array([speed_of(w) >= MIN_SPEED for w in X])


# ════════════════════════════════════════════════════════
# 채널별 값을 조건 칸 기준으로 z 화하는 공용 루틴
#
# 학습 쪽(edge_node)과 평가 쪽(valset)이 같은 자를 써야 라운드별
# 숫자와 운영 판정이 어긋나지 않는다. 그래서 여기 한 곳에 둔다.
#
# [MAD 하한이 왜 필요한가]
# 어떤 채널은 캡처 내내 상수다(예: 적재 안 하는 주행의 lift.pos).
# 그 채널의 MAD 는 0 이고, 0 에 가까운 값으로 나누면 z 가 10^7 까지
# 튄다. 실제로 그래서 모든 사업장이 똑같이 "이상" 으로 보였다.
# 두 가지로 막는다.
#   - 전체가 상수인 채널은 아예 뺀다(z=0). 정보가 없는 채널이다.
#   - 칸의 MAD 는 그 채널 전체 MAD 의 일정 비율 아래로 못 내려간다.
#     칸 표본이 적어 우연히 몰려 있을 때 터지는 것을 막는다.
# ════════════════════════════════════════════════════════

MAD_K = 1.4826          # 정규분포에서 MAD 를 표준편차와 같은 자로
MAD_FLOOR_REL = 0.1     # 칸 MAD 하한 = 채널 전체 MAD 의 10%


def _mad(A, axis=0):
    med = np.median(A, axis=axis)
    return med, np.median(np.abs(A - med), axis=axis) * MAD_K


def robust_ref(D, keys, min_cell: int = 8):
    """채널별 값 (N, C) -> (칸별 기준, 전체 기준, 살아있는 채널 마스크).

    D    : (N, C) 창마다 채널마다 하나씩인 값
    keys : (N,)   조건 칸 번호
    반환 : ({칸: (중앙값, MAD)}, (중앙값, MAD), 유효채널 bool (C,))
    """
    D = np.asarray(D, dtype=np.float64)
    g_med, g_mad = _mad(D)
    alive = g_mad > 0                      # 전체가 상수인 채널은 버린다
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
    """robust_ref 의 기준으로 z 화 (N, C). 죽은 채널은 0 으로 둔다."""
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
# 사업장별 입력 정규화 통계
#
# [왜 R1 에서 고정하면 안 되나]
# 예전에는 첫 수집분(약 50창)으로 평균/표준편차를 잡고 끝까지 썼다.
# 아이작심은 빈 차로 출발하므로 첫 수집은 거의 다 빈 차다. 실측
# (2026-11-19 01:19 실행, siteA): R1 은 45/45 빈 차, R6 은 0/50 빈 차.
# R1 의 lift.pos 표준편차가 6.7e-5 라 적재 구간이 오면 |z| 가 40~80
# 까지 튀었고, 학습 손실이 1.1 -> 14.3 으로 오른 것은 학습이 망가진
# 게 아니라 이 자가 어긋난 것이었다(재현한 평균 z² 와 손실이 거의 같다).
#
# [어떻게 하나]
#   - 적재 상태(빈/가벼움/무거움)가 각각 NORM_MIN_PER_LOAD 창 이상 모일
#     때까지는 받은 창을 쌓아 가며 통계를 다시 잡는다.
#   - 다 모였거나 NORM_FREEZE_ROUND 에 이르면 고정한다. 그 뒤로 자가
#     바뀌면 라운드끼리 점수를 비교할 수 없다.
#   - 표준편차 하한과 |z| 클리핑을 같이 건다. 고정 전에도, 희귀한 창
#     하나가 손실을 지배하지 못하게 하는 안전장치다.
#
# 같은 telemetry 로 재현한 라운드별 평균 z² (siteA):
#   R1 고정        1.0 2.9 1.4 5.4 13.6 16.7 16.8 15.4 10.8 5.1
#   누적 후 고정    1.0 1.2 0.6 0.6  0.7  0.6  0.5  2.0  0.8 0.6  (R5 에 고정)
# ════════════════════════════════════════════════════════

NORM_MIN_PER_LOAD = int(os.getenv("AMR_NORM_MIN_PER_LOAD", "20"))
NORM_FREEZE_ROUND = int(os.getenv("AMR_NORM_FREEZE_ROUND", "5"))
NORM_STD_FLOOR = float(os.getenv("AMR_NORM_STD_FLOOR", "1e-4"))
NORM_CLIP = float(os.getenv("AMR_NORM_CLIP", "10"))
# 쌓아 둘 원신호 창의 상한. 고정 전까지만 쓰고 고정하면 버린다.
NORM_MAX_BUFFER = int(os.getenv("AMR_NORM_MAX_BUFFER", "2000"))


class NormStats:
    """사업장 하나의 채널별 정규화 통계. 고정될 때까지 누적한다."""

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
        """cond 벡터 (N, COND_DIM) -> 적재 상태 인덱스 (N,)."""
        C = np.asarray(cond)
        lo = N_NAV + N_SPD
        if C.ndim != 2 or C.shape[1] < lo + N_LOAD:
            return np.zeros(len(C), dtype=np.int64)
        return np.argmax(C[:, lo:lo + N_LOAD], axis=1).astype(np.int64)

    def update(self, X: np.ndarray, cond: Optional[np.ndarray],
               round_num: int) -> None:
        """이번 라운드의 원신호 창 (N, C, T) 로 통계를 갱신한다. 고정됐으면 무시."""
        if self.frozen or X is None or len(X) == 0:
            return
        X = np.asarray(X, dtype=np.float32)
        self._buf.append(X)
        if cond is not None and len(cond) == len(X):
            self._load_counts += np.bincount(self.load_of(cond),
                                             minlength=N_LOAD)[:N_LOAD]

        A = np.concatenate(self._buf)
        if len(A) > self.max_buffer:
            # 넘치면 시간축 전체에서 고르게 남긴다.
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
        """(N, C, T) 원신호 -> 표준화 + 클리핑."""
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
