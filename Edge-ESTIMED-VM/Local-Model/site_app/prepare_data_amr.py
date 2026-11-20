"""
prepare_data_amr.py — AMR 주행 데이터를 FL 노드(사업장)별 PKL로 변환

입력 형식 두 가지를 모두 받는다.

  1) CIN 덤프 (새 형식, 권장)
     <robot>/telemetry.json  — JSON Lines. 한 줄이 contentInstance 하나.
       {"ri":…, "rn":…, "ct":…, "lbl":null, "con":{…}}
     디렉터리를 주면 그 아래 */telemetry.json 을 모두 읽는다.

  2) merged.json (옛 형식)
     run 목록이고 run 하나가 CIN 여러 개를 갖는다. lbl에 scn/spd/load가 있다.

출력: /tmp/fl_data/amr/{mn1,mn2,mn3}.pkl

[라벨에 대하여]
운영 중에는 cond:normal 같은 라벨이 붙지 않는다. 로봇은 그냥 주행하고
데이터를 올릴 뿐이고, 이상 탐지는 정상 주행만 학습하는 비지도 방식이다.
그래서 이 스크립트도 라벨을 요구하지 않는다.

평가용 정답(over_bump 등)은 학습에 쓰지 않고 AUROC를 재는 데만 쓴다.
시뮬레이터가 아는 결함 위치를 결과 확인용으로만 쓰는 것과 같다.

[정지 구간]
nav 라벨이 아니라 실제 주행 속도로 가려낸다. 그래야 주행 중 잠깐 멈춘
구간은 살아남고 진짜 정지만 빠진다. 기준은 AMR_MIN_SPEED.

[중단된 주행]
nav=aborted 는 중단된 주행이라 정상 주행으로 보기 어렵다. 학습에서는 빼고
평가에는 남긴다. AMR_EXCLUDE_NAV 로 조정한다.

[사업장 분할]
로봇 식별자로 나눈다. A 4대 / B 2대 / C 4대.
siteA_* / siteB_* / siteC_* 규칙을 따르거나 AMR_ROBOT_MAP으로 지정한다.
"""

from __future__ import annotations

import glob
import json
import os
import pickle
import sys

import numpy as np

import amr_schema as sch

# 하위 호환: 기존 코드가 prepare_data_amr.CH_SPEC 등을 참조한다.
CH_SPEC = sch.CH_SPEC
CH_NAMES = sch.CH_NAMES
IS_POS = sch.IS_POS
N_CH = sch.N_CH
T = sch.T
COND_DIM = sch.COND_DIM
cond_vector = sch.cond_vector

SRC = os.getenv(
    "AMR_DATA",
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
                 "amr_data"),
)
OUT_DIR = os.getenv("AMR_OUT_DIR", "/tmp/fl_data/amr")

NORMAL = "normal"
ANOMALY_NAV = {"over_bump"}          # 평가 정답으로 쓸 nav

NODES = ["mn1", "mn2", "mn3"]
SITE_OF_NODE = {"mn1": "siteA", "mn2": "siteB", "mn3": "siteC"}
NODE_OF_SITE = {v: k for k, v in SITE_OF_NODE.items()}
EXPECTED_ROBOTS = {"mn1": 4, "mn2": 2, "mn3": 2}

VAL_RATIO = float(os.getenv("AMR_VAL_RATIO", "0.2"))

# 정상 구간 중 시험 전용으로 떼어 둘 비율.
# 검증(임계값 보정)과 시험(평가)이 겹치면 평가가 독립적이지 않다.
TEST_NORMAL_RATIO = float(os.getenv("AMR_TEST_NORMAL_RATIO", "0.2"))

# 블록 경계에서 버릴 윈도우 수.
# 1초 윈도우가 연속이라 경계를 맞댄 두 윈도우는 사실상 같은 장면이다.
EMBARGO = int(os.getenv("AMR_EMBARGO", "2"))

# run 단위로 나누려면 run이 최소 몇 개 있어야 하는가.
# 4개면 학습 2 / 검증 1 / 시험 1 이 된다. 그보다 적으면 run 안에서 자른다.
MIN_RUNS_FOR_RUN_SPLIT = int(os.getenv("AMR_MIN_RUNS", "4"))

# 사업장당 정상 윈도우 상한.
#
# 사업장마다 로봇 수와 주행량이 달라 총량이 어긋난다. FedAvg가 샘플 수로
# 가중하므로 많이 가진 사업장이 더 큰 영향력을 갖는다. 사업장별 기여를
# 맞추려면 여기서 잘라야 한다.
#
#   "0"    자르지 않는다 (기본)
#   "min"  가장 적은 사업장에 맞춘다
#   숫자   그 값으로 맞춘다
#
# 기본이 끔인 이유는, 한 사업장의 수집이 덜 됐을 때 "min"이 나머지를
# 거기에 맞춰 조용히 날려버리기 때문이다. 사업장이 고르게 모인 것을
# 확인한 뒤에 켜는 편이 안전하다.
#
# 이상 구간은 절대 자르지 않는다. 단차가 수십 개뿐이라 건드리면 평가가
# 불가능해진다. 상한은 정상 윈도우에만 적용한다.
MAX_NORMAL_PER_SITE = os.getenv("AMR_MAX_NORMAL_PER_SITE", "0").strip()

# "min"이 안전한지 판단하는 기준. 가장 적은 사업장이 가장 많은 사업장의
# 이 비율보다 작으면, 맞추는 것이 득보다 실이 크다고 보고 경고한다.
MIN_BALANCE_RATIO = float(os.getenv("AMR_MIN_BALANCE_RATIO", "0.5"))
SEED = int(os.getenv("AMR_SEED", "42"))

_fallback_warned = False


def _parse_robot_map(spec: str) -> dict:
    out = {}
    for part in spec.split(";"):
        if "=" not in part:
            continue
        node, robots = part.split("=", 1)
        for r in robots.split(","):
            if r.strip():
                out[r.strip()] = node.strip()
    return out


ROBOT_MAP = _parse_robot_map(os.getenv("AMR_ROBOT_MAP", ""))


def site_of_robot(robot: str) -> str:
    global _fallback_warned
    if robot in ROBOT_MAP:
        return ROBOT_MAP[robot]
    prefix = robot.split("_", 1)[0] if "_" in robot else ""
    if prefix in NODE_OF_SITE:
        return NODE_OF_SITE[prefix]
    if not _fallback_warned:
        print(f"  ⚠ 로봇 식별자로 사업장을 정할 수 없다 (예: {robot!r}) -> mn1로 보낸다.")
        print("    siteA_* / siteB_* / siteC_* 규칙을 쓰거나 AMR_ROBOT_MAP을 지정하라.")
        _fallback_warned = True
    return "mn1"


# ── 입력 읽기 ─────────────────────────────────────────────
def _iter_cin_dump(path: str):
    """JSON Lines CIN 덤프에서 (con, lbl)을 흘려준다."""
    with open(path, encoding="utf-8") as fp:
        for line in fp:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except ValueError:
                continue
            con = rec.get("con")
            if isinstance(con, str):
                try:
                    con = json.loads(con)
                except ValueError:
                    continue
            if isinstance(con, dict):
                yield con, (rec.get("lbl") or [])


def _iter_merged(path: str):
    """옛 merged.json에서 (con, lbl)을 흘려준다. run 라벨을 CIN에 얹는다."""
    with open(path, encoding="utf-8") as fp:
        data = json.load(fp)
    runs = data if isinstance(data, list) else data.get("runs", [])
    for run in runs:
        run_lbl = list(run.get("lbl", []))
        for cin in run.get("cins", []):
            con = cin.get("con")
            if isinstance(con, str):
                try:
                    con = json.loads(con)
                except ValueError:
                    continue
            if isinstance(con, dict):
                yield con, (cin.get("lbl") or run_lbl)


def iter_records(src: str):
    """파일/디렉터리 어느 쪽이든 (con, lbl)을 흘려준다."""
    if os.path.isdir(src):
        files = sorted(glob.glob(os.path.join(src, "**", "telemetry.json"),
                                 recursive=True))
        if files:
            for f in files:
                yield from _iter_cin_dump(f)
            return
        merged = os.path.join(src, "merged.json")
        if os.path.exists(merged):
            yield from _iter_merged(merged)
            return
        raise FileNotFoundError(f"{src} 아래에서 telemetry.json 도 merged.json 도 못 찾았다")

    name = os.path.basename(src).lower()
    if name.startswith("merged"):
        yield from _iter_merged(src)
    else:
        yield from _iter_cin_dump(src)


# ── 파싱 ──────────────────────────────────────────────────
def parse(src: str) -> dict:
    X, C, NAV, ROBOT, POSE, NODE, LABEL = [], [], [], [], [], [], []
    # run 경계를 찾고 시간순으로 정렬하려면 순서 정보가 필요하다.
    SEQ, TSIM = [], []
    skipped = 0

    for con, lbl in iter_records(src):
        sig = sch.window_from_con(con)
        if sig is None:
            skipped += 1
            continue

        labels = {x.split(":")[0]: x.split(":", 1)[1]
                  for x in lbl if isinstance(x, str) and ":" in x}

        nav = con.get("nav") or labels.get("scn") or "idle"
        speed = sch.speed_of(sig)

        # 조건 벡터: 라벨이 있으면 쓰고, 없으면 payload에서 끌어낸다.
        if "scn" in labels and "spd" in labels:
            cvec = sch.cond_from_labels(labels["scn"], labels["spd"])
        else:
            cvec = sch.cond_vector(nav, speed, sig)

        # 평가용 정답. 라벨이 있으면 라벨 우선, 없으면 nav로 판단.
        cond_name = labels.get(
            "cond",
            "floor_bump" if nav in ANOMALY_NAV else NORMAL,
        )

        X.append(sig)
        C.append(cvec)
        NAV.append(nav)
        ROBOT.append(con.get("robot", "unknown"))
        POSE.append(sch.pose_of(con))
        NODE.append(site_of_robot(con.get("robot", "unknown")))
        LABEL.append(cond_name)

        # seq는 로봇별 CIN 순번, t_sim은 시뮬레이션 시각.
        # 둘 다 주행이 다시 시작되면 되돌아간다.
        SEQ.append(_as_int(con.get("seq")))
        ts = con.get("t_sim")
        TSIM.append(float(ts[0]) if isinstance(ts, list) and ts
                    else _as_float(ts))

    return {
        "X": np.asarray(X, dtype=np.float32),
        "C": np.asarray(C, dtype=np.float32),
        "nav": np.asarray(NAV),
        "robot": np.asarray(ROBOT),
        "pose": np.asarray(POSE, dtype=np.float32),
        "node": np.asarray(NODE),
        "label": np.asarray(LABEL),
        "seq": np.asarray(SEQ, dtype=np.int64),
        "t_sim": np.asarray(TSIM, dtype=np.float64),
        "skipped": skipped,
    }


# ── run 식별과 분할 ───────────────────────────────────────

def _as_int(v) -> int:
    try:
        return int(v)
    except (TypeError, ValueError):
        return -1


def _as_float(v) -> float:
    try:
        return float(v)
    except (TypeError, ValueError):
        return float("nan")


def derive_run_ids(robot: np.ndarray, seq: np.ndarray,
                   t_sim: np.ndarray) -> np.ndarray:
    """
    윈도우마다 run(주행) 식별자를 붙인다.

    새 run으로 보는 경우
      - 로봇이 바뀐다
      - seq가 줄거나 같다 (주행이 다시 시작돼 순번이 리셋됨)
      - t_sim이 뒤로 간다 (시뮬레이션 시각이 되감김)

    지금 데이터는 로봇 하나가 연속 주행 하나라 run = 로봇이 된다.
    나중에 한 로봇이 여러 번 주행한 덤프가 와도 그대로 동작한다.
    """
    n = len(robot)
    if n == 0:
        return np.zeros(0, dtype=np.int64)

    run = np.zeros(n, dtype=np.int64)
    cur = 0
    for i in range(1, n):
        new_run = robot[i] != robot[i - 1]
        if not new_run and seq[i] >= 0 and seq[i - 1] >= 0:
            new_run = seq[i] <= seq[i - 1]
        if not new_run and np.isfinite(t_sim[i]) and np.isfinite(t_sim[i - 1]):
            new_run = t_sim[i] < t_sim[i - 1]
        if new_run:
            cur += 1
        run[i] = cur
    return run


def _blocks_in_run(order: np.ndarray) -> tuple:
    """
    한 run 안을 시간순 연속 블록 셋으로 자른다.
      [학습][embargo][검증][embargo][시험]

    무작위로 섞지 않는 이유는, 이웃한 1초 윈도우가 사실상 같은 장면이라
    섞으면 학습과 검증이 같은 장면을 나눠 갖기 때문이다.
    embargo는 블록이 맞닿는 지점의 윈도우를 버려서 그 누수를 막는다.
    """
    n = len(order)
    if n == 0:
        return order[:0], order[:0], order[:0]

    n_test = int(n * TEST_NORMAL_RATIO)
    n_val = int(n * VAL_RATIO)

    # 너무 짧으면 나누지 않고 전부 학습으로 둔다.
    if n < 3 * (EMBARGO + 1) or n_val < 1 or n_test < 1:
        return order, order[:0], order[:0]

    n_train = n - n_val - n_test - 2 * EMBARGO
    if n_train < 1:
        return order, order[:0], order[:0]

    a = n_train
    b = a + EMBARGO
    c = b + n_val
    d = c + EMBARGO
    return order[:a], order[b:c], order[d:]


def balance_indices(idx: np.ndarray, robot: np.ndarray,
                    cap: int) -> np.ndarray:
    """
    인덱스를 cap개로 줄이되 로봇별로 고르게 남긴다.

    로봇마다 cap/로봇수 만큼 배정하고, 적게 가진 로봇의 몫은 나머지가
    나눠 갖는다. 앞에서 자르면 주행 초반만 남으므로 시간 축 전체에서
    균등 간격으로 뽑는다.
    """
    if cap <= 0 or len(idx) <= cap:
        return idx

    uniq = sorted(set(robot[idx].tolist()))
    by_robot = {r: idx[robot[idx] == r] for r in uniq}

    quota = {r: 0 for r in uniq}
    remaining, pool = cap, list(uniq)
    while remaining > 0 and pool:
        share = max(1, remaining // len(pool))
        for r in list(pool):
            take = min(share, len(by_robot[r]) - quota[r], remaining)
            quota[r] += take
            remaining -= take
            if quota[r] >= len(by_robot[r]):
                pool.remove(r)
            if remaining <= 0:
                break

    keep = []
    for r in uniq:
        src = np.sort(by_robot[r])
        k = quota[r]
        if k > 0:
            keep.append(src[np.linspace(0, len(src) - 1, k).astype(np.int64)])
    return np.sort(np.concatenate(keep)) if keep else idx[:0]


def split_normals(idx: np.ndarray, run: np.ndarray,
                  rng: np.random.Generator) -> tuple:
    """
    정상 윈도우 인덱스를 학습 / 검증 / 시험으로 나눈다.

    run이 넉넉하면 run 통째로 배정한다. 서로 다른 주행이므로 누수가 없다.
    부족하면 run 안에서 시간순 블록으로 자른다.
    반환에 어느 방식을 썼는지도 같이 준다.
    """
    if len(idx) == 0:
        e = idx[:0]
        return e, e, e, "없음"

    runs = np.unique(run[idx])

    if len(runs) >= MIN_RUNS_FOR_RUN_SPLIT:
        order = rng.permutation(runs)
        n_val = max(1, int(round(len(runs) * VAL_RATIO)))
        n_test = max(1, int(round(len(runs) * TEST_NORMAL_RATIO)))
        # 학습 run이 최소 하나는 남아야 한다.
        while n_val + n_test >= len(runs) and n_val + n_test > 2:
            if n_val >= n_test:
                n_val -= 1
            else:
                n_test -= 1

        val_runs = set(order[:n_val].tolist())
        test_runs = set(order[n_val:n_val + n_test].tolist())

        in_val = np.isin(run[idx], list(val_runs))
        in_test = np.isin(run[idx], list(test_runs))
        return (idx[~(in_val | in_test)], idx[in_val], idx[in_test],
                f"run 단위 ({len(runs)}개 run: 학습 "
                f"{len(runs) - n_val - n_test} / 검증 {n_val} / 시험 {n_test})")

    tr, va, te = [], [], []
    for r in runs:
        order = idx[run[idx] == r]
        order = order[np.argsort(order)]      # 시간순
        a, b, c = _blocks_in_run(order)
        tr.append(a); va.append(b); te.append(c)

    cat = lambda parts: (np.concatenate(parts) if parts else idx[:0])
    return (cat(tr), cat(va), cat(te),
            f"run 내 시간순 블록 ({len(runs)}개 run, embargo {EMBARGO})")


# ── 노드별 PKL ────────────────────────────────────────────
def build_node_pkl(d: dict, node: str, rng: np.random.Generator,
                   normal_cap: int = 0) -> dict:
    sel = d["node"] == node
    if not sel.any():
        return {}

    X, C = d["X"][sel], d["C"][sel]
    nav, robot = d["nav"][sel], d["robot"][sel]
    pose, label = d["pose"][sel], d["label"][sel]
    seq, t_sim = d["seq"][sel], d["t_sim"][sel]

    # 주행 단위. 누수 없이 나누려면 구간이 아니라 주행으로 갈라야 한다.
    run = derive_run_ids(robot, seq, t_sim)

    # 학습 후보: 움직이는 구간 + 이상 아님 + 제외 nav 아님
    moving = sch.moving_mask(X)
    is_normal = label == NORMAL
    not_excluded = ~np.isin(nav, list(sch.EXCLUDE_NAV_TRAIN))
    trainable = moving & is_normal & not_excluded

    # 사업장별 총량을 맞춘다. 이상 구간은 건드리지 않는다.
    train_pool = np.where(trainable)[0]
    if normal_cap > 0 and len(train_pool) > normal_cap:
        before = len(train_pool)
        train_pool = balance_indices(train_pool, robot, normal_cap)
        print(f"    [{node}] 사업장 상한: 정상 {before} -> "
              f"{len(train_pool)}개 (로봇별 균등)")

    # 정상을 셋으로 나눈다. 검증은 임계값 보정, 시험은 평가 전용이다.
    tr_idx, val_idx, te_norm_idx, how = split_normals(train_pool, run, rng)

    tr = np.zeros(len(X), bool); tr[tr_idx] = True
    va = np.zeros(len(X), bool); va[val_idx] = True
    te_norm = np.zeros(len(X), bool); te_norm[te_norm_idx] = True

    # 평가 = 시험 전용 정상 + 이상 + 중단.
    # 검증 구간(va)은 넣지 않는다. 임계값을 맞춘 데이터로 점수를 매기면
    # 독립적인 평가가 아니다.
    te = moving & (te_norm | ~is_normal | ~not_excluded)

    print(f"    [{node}] 분할 방식: {how}")
    if va.any() and not te_norm.any():
        print(f"    [{node}] ⚠ 시험용 정상 구간이 없다. "
              f"AMR_TEST_NORMAL_RATIO를 올리거나 데이터를 늘려라.")

    if tr.any():
        mean = X[tr].mean(axis=(0, 2), keepdims=True)
        std = X[tr].std(axis=(0, 2), keepdims=True)
    else:
        mean = np.zeros((1, N_CH, 1), np.float32)
        std = np.ones((1, N_CH, 1), np.float32)
    std = std + 1e-8

    def norm(a):
        return ((a - mean) / std).astype(np.float32)

    return {
        "train_signals": norm(X[tr]), "train_cond": C[tr],
        "train_nav": nav[tr], "train_robot": robot[tr],
        "train_run_id": run[tr],

        "val_signals": norm(X[va]), "val_cond": C[va],
        "val_labels": np.zeros(int(va.sum()), dtype=np.int64),
        "val_nav": nav[va], "val_run_id": run[va],

        "test_signals": norm(X[te]), "test_cond": C[te],
        "test_labels": (label[te] != NORMAL).astype(np.int64),
        "test_label_name": label[te], "test_nav": nav[te],
        "test_pose": pose[te], "test_robot": robot[te],
        "test_run_id": run[te],
        "split_method": how,

        "norm_mean": mean.astype(np.float32),
        "norm_std": std.astype(np.float32),
        "channel_names": CH_NAMES, "cond_dim": COND_DIM,
        "n_channels": N_CH, "seq_len": T,
        "node": node, "site": SITE_OF_NODE.get(node, node),
        "robots": sorted(set(robot.tolist())),
        "spd_edges": list(sch.SPD_EDGES), "min_speed": sch.MIN_SPEED,
    }


def main() -> int:
    if not os.path.exists(SRC):
        print(f"✗ 데이터가 없다: {SRC}")
        print("  AMR_DATA 로 telemetry.json 또는 그 상위 디렉터리를 지정하라.")
        return 1

    print(f"입력: {SRC}")
    d = parse(SRC)
    if not len(d["X"]):
        print("✗ 유효한 윈도우가 없다")
        return 1

    moving = sch.moving_mask(d["X"])
    print(f"  윈도우 {d['X'].shape}  (건너뜀 {d['skipped']}개)")
    print(f"  이동 {int(moving.sum())} / 정지 {int((~moving).sum())}"
          f"  (기준 속도 {sch.MIN_SPEED})")
    u, c = np.unique(d["label"], return_counts=True)
    print("  상태별:", dict(zip(u.tolist(), c.tolist())))
    u, c = np.unique(d["robot"], return_counts=True)
    print("  로봇별:", dict(zip(u.tolist(), c.tolist())))

    os.makedirs(OUT_DIR, exist_ok=True)
    rng = np.random.default_rng(SEED)

    # 사업장별 정상 윈도우 수를 먼저 세고 상한을 정한다.
    # "min"이면 가장 적은 사업장에 맞춘다.
    normal_cap = 0
    counts = {}
    for node in NODES:
        sel = d["node"] == node
        if not sel.any():
            continue
        X = d["X"][sel]
        ok = (sch.moving_mask(X)
              & (d["label"][sel] == NORMAL)
              & ~np.isin(d["nav"][sel], list(sch.EXCLUDE_NAV_TRAIN)))
        counts[node] = int(ok.sum())

    if counts:
        if MAX_NORMAL_PER_SITE.lower() == "min":
            normal_cap = min(counts.values())
        elif MAX_NORMAL_PER_SITE.isdigit():
            normal_cap = int(MAX_NORMAL_PER_SITE)

        lo, hi = min(counts.values()), max(counts.values())
        ratio = (lo / hi) if hi else 1.0

        print(f"  사업장별 정상: "
              + " / ".join(f"{SITE_OF_NODE.get(n, n)} {c}"
                           for n, c in counts.items())
              + f"   (최소/최대 {ratio:.2f})")

        if normal_cap > 0:
            if normal_cap < hi * MIN_BALANCE_RATIO:
                print(f"  ⚠ 상한 {normal_cap}이 가장 많은 사업장({hi})의 "
                      f"{normal_cap / hi * 100:.0f}%밖에 안 된다.")
                print(f"    맞추려다 데이터 대부분을 버리게 된다. "
                      f"수집이 고르게 될 때까지 끄는 편이 낫다.")
                print(f"    그래도 쓰려면 AMR_MIN_BALANCE_RATIO 를 낮춰라. "
                      f"지금은 상한을 적용하지 않는다.")
                normal_cap = 0
            else:
                print(f"  → 상한 {normal_cap}개로 맞춘다 "
                      f"(AMR_MAX_NORMAL_PER_SITE={MAX_NORMAL_PER_SITE})")
        else:
            print("  → 상한 없음 (맞추려면 AMR_MAX_NORMAL_PER_SITE=min)")
            if ratio < MIN_BALANCE_RATIO:
                print(f"    ⓘ 사업장 간 차이가 크다. FedAvg가 샘플 수로 "
                      f"가중하므로 많이 가진 사업장의 영향이 커진다.")

    for node in NODES:
        pkl = build_node_pkl(d, node, rng, normal_cap)
        if not pkl:
            print(f"  ⚠ {node}({SITE_OF_NODE[node]}): 배정된 데이터가 없다")
            continue
        want = EXPECTED_ROBOTS.get(node)
        n_rb = len(pkl["robots"])
        warn = "" if (want is None or n_rb == want) else f" ⚠예상{want}대"
        path = os.path.join(OUT_DIR, f"{node}.pkl")
        with open(path, "wb") as fp:
            pickle.dump(pkl, fp)
        print(f"  ✓ {node}({pkl['site']}) 로봇{n_rb}대{warn}: "
              f"학습 {len(pkl['train_signals'])} / 검증 {len(pkl['val_signals'])} / "
              f"평가 {len(pkl['test_signals'])} (이상 {int(pkl['test_labels'].sum())})")

    return 0


if __name__ == "__main__":
    sys.exit(main())
