"""
prepare_data_amr.py — converts AMR driving data into PKL per FL node (site)

Accepts both input formats.

  1) CIN dump (new format, recommended)
     <robot>/telemetry.json  — JSON Lines. One line is one contentInstance.
       {"ri":…, "rn":…, "ct":…, "lbl":null, "con":{…}}
     Given a directory, reads every */telemetry.json under it.

  2) merged.json (old format)
     A list of runs, each run holding several CINs. lbl has scn/spd/load.

Output: /tmp/fl_data/amr/{mn1,mn2,mn3}.pkl

[About labels]
In operation there are no labels like cond:normal. The robots just drive and
upload data, and anomaly detection is unsupervised, trained only on normal driving.
So this script does not require labels either.

Evaluation ground truth (over_bump etc.) is not used for training, only for measuring AUROC.
It is like using the defect locations the simulator knows only to check results.

[Stopped segments]
Filtered by actual driving speed, not the nav label. That way brief stops while driving
survive and only real stops are dropped. The threshold is AMR_MIN_SPEED.

[Aborted runs]
nav=aborted is an aborted run and hard to treat as normal driving. It is excluded from training
and kept for evaluation. Adjust with AMR_EXCLUDE_NAV.

[Site split]
Split by robot identifier. A: 4 / B: 2 / C: 4.
Follows the siteA_* / siteB_* / siteC_* rule, or set via AMR_ROBOT_MAP.
"""

from __future__ import annotations

import glob
import json
import os
import pickle
import sys

import numpy as np

import amr_schema as sch

# Backward compatibility: existing code references prepare_data_amr.CH_SPEC etc.
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
ANOMALY_NAV = {"over_bump"}          # nav values used as evaluation ground truth

NODES = ["mn1", "mn2", "mn3"]
SITE_OF_NODE = {"mn1": "siteA", "mn2": "siteB", "mn3": "siteC"}
NODE_OF_SITE = {v: k for k, v in SITE_OF_NODE.items()}
EXPECTED_ROBOTS = {"mn1": 4, "mn2": 2, "mn3": 2}

VAL_RATIO = float(os.getenv("AMR_VAL_RATIO", "0.2"))

# Fraction of normal segments set aside for testing only.
# If validation (threshold calibration) and test (evaluation) overlap, the evaluation is not independent.
TEST_NORMAL_RATIO = float(os.getenv("AMR_TEST_NORMAL_RATIO", "0.2"))

# Number of windows to drop at block boundaries.
# 1 s windows are contiguous, so two windows touching at a boundary are effectively the same scene.
EMBARGO = int(os.getenv("AMR_EMBARGO", "2"))

# Minimum number of runs needed to split by run.
# With 4 it becomes train 2 / val 1 / test 1. With fewer, it splits within a run.
MIN_RUNS_FOR_RUN_SPLIT = int(os.getenv("AMR_MIN_RUNS", "4"))

# Cap on normal windows per site.
#
# Robot count and driving volume differ per site, so totals are uneven. FedAvg weights by sample count,
# so the site with more data gets more influence. To balance per-site contributions
# it must be cut here.
#
#   "0"    no cutting (default)
#   "min"  match the smallest site
#   number match to that value
#
# The default is off because when one site's collection is incomplete, "min" silently cuts
# the others down to it. It is safer to turn it on after confirming the sites
# collected evenly.
#
# Anomalous segments are never cut. There are only a few dozen bumps, so touching them makes
# evaluation impossible. The cap applies only to normal windows.
MAX_NORMAL_PER_SITE = os.getenv("AMR_MAX_NORMAL_PER_SITE", "0").strip()

# Criterion for whether "min" is safe. If the smallest site is below this fraction of the
# largest site, matching is judged to do more harm than good and a warning is issued.
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
        print(f"  ⚠ cannot map robot ID to a site (e.g. {robot!r}) -> sending to mn1.")
        print("    Use the siteA_* / siteB_* / siteC_* naming or set AMR_ROBOT_MAP.")
        _fallback_warned = True
    return "mn1"


# ── Read input ────────────────────────────────────────────
def _iter_cin_dump(path: str):
    """Stream (con, lbl) from a JSON Lines CIN dump."""
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
    """Stream (con, lbl) from the old merged.json. The run label is attached to each CIN."""
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
    """Stream (con, lbl) from either a file or a directory."""
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
        raise FileNotFoundError(f"neither telemetry.json nor merged.json found under {src}")

    name = os.path.basename(src).lower()
    if name.startswith("merged"):
        yield from _iter_merged(src)
    else:
        yield from _iter_cin_dump(src)


# ── Parse ─────────────────────────────────────────────────
def parse(src: str) -> dict:
    X, C, NAV, ROBOT, POSE, NODE, LABEL = [], [], [], [], [], [], []
    # Ordering info is needed to find run boundaries and sort by time.
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

        # Condition vector: use labels if present, otherwise derive from the payload.
        if "scn" in labels and "spd" in labels:
            cvec = sch.cond_from_labels(labels["scn"], labels["spd"])
        else:
            cvec = sch.cond_vector(nav, speed, sig)

        # Evaluation ground truth. Labels take priority if present; otherwise decided from nav.
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

        # seq is the per-robot CIN sequence number, t_sim is simulation time.
        # Both reset when driving restarts.
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


# ── Run identification and split ──────────────────────────

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
    Attach a run (driving session) identifier to each window.

    Treated as a new run when
      - the robot changes
      - seq decreases or stays the same (driving restarted and the counter reset)
      - t_sim goes backward (simulation time rewound)

    In the current data each robot does one continuous run, so run = robot.
    It still works if a later dump has several runs per robot.
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
    Cut one run into three contiguous blocks in time order.
      [train][embargo][val][embargo][test]

    They are not shuffled because neighboring 1 s windows are effectively the same scene,
    and shuffling would let training and validation share the same scenes.
    The embargo drops windows where blocks meet to prevent that leakage.
    """
    n = len(order)
    if n == 0:
        return order[:0], order[:0], order[:0]

    n_test = int(n * TEST_NORMAL_RATIO)
    n_val = int(n * VAL_RATIO)

    # If too short, it is not split and everything goes to training.
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
    Reduce the indices to cap, keeping them even across robots.

    Each robot is allotted cap / number of robots, and the share of robots with fewer is split among
    the rest. Cutting from the front would keep only the start of the run, so sample at even intervals
    across the whole time axis.
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
    Split normal window indices into train / val / test.

    With enough runs, assign whole runs. They are different sessions, so there is no leakage.
    If not enough, cut within each run into time-ordered blocks.
    Also returns which method was used.
    """
    if len(idx) == 0:
        e = idx[:0]
        return e, e, e, "none"

    runs = np.unique(run[idx])

    if len(runs) >= MIN_RUNS_FOR_RUN_SPLIT:
        order = rng.permutation(runs)
        n_val = max(1, int(round(len(runs) * VAL_RATIO)))
        n_test = max(1, int(round(len(runs) * TEST_NORMAL_RATIO)))
        # At least one training run must remain.
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
                f"by run ({len(runs)} runs: train "
                f"{len(runs) - n_val - n_test} / val {n_val} / test {n_test})")

    tr, va, te = [], [], []
    for r in runs:
        order = idx[run[idx] == r]
        order = order[np.argsort(order)]      # Time order
        a, b, c = _blocks_in_run(order)
        tr.append(a); va.append(b); te.append(c)

    cat = lambda parts: (np.concatenate(parts) if parts else idx[:0])
    return (cat(tr), cat(va), cat(te),
            f"time-ordered blocks within runs ({len(runs)} runs, embargo {EMBARGO})")


# ── Per-node PKL ──────────────────────────────────────────
def build_node_pkl(d: dict, node: str, rng: np.random.Generator,
                   normal_cap: int = 0) -> dict:
    sel = d["node"] == node
    if not sel.any():
        return {}

    X, C = d["X"][sel], d["C"][sel]
    nav, robot = d["nav"][sel], d["robot"][sel]
    pose, label = d["pose"][sel], d["label"][sel]
    seq, t_sim = d["seq"][sel], d["t_sim"][sel]

    # Run units. To split without leakage, split by run, not by segment.
    run = derive_run_ids(robot, seq, t_sim)

    # Training candidates: moving segments + not anomalous + not an excluded nav
    moving = sch.moving_mask(X)
    is_normal = label == NORMAL
    not_excluded = ~np.isin(nav, list(sch.EXCLUDE_NAV_TRAIN))
    trainable = moving & is_normal & not_excluded

    # Balance per-site totals. Anomalous segments are not touched.
    train_pool = np.where(trainable)[0]
    if normal_cap > 0 and len(train_pool) > normal_cap:
        before = len(train_pool)
        train_pool = balance_indices(train_pool, robot, normal_cap)
        print(f"    [{node}] site cap: normal {before} -> "
              f"{len(train_pool)} (even per robot)")

    # Split normal data into three. Validation is for threshold calibration; test is for evaluation only.
    tr_idx, val_idx, te_norm_idx, how = split_normals(train_pool, run, rng)

    tr = np.zeros(len(X), bool); tr[tr_idx] = True
    va = np.zeros(len(X), bool); va[val_idx] = True
    te_norm = np.zeros(len(X), bool); te_norm[te_norm_idx] = True

    # Evaluation = test-only normal + anomalies + aborted.
    # The validation segment (va) is not included. Scoring on the data used to tune the threshold
    # is not an independent evaluation.
    te = moving & (te_norm | ~is_normal | ~not_excluded)

    print(f"    [{node}] split method: {how}")
    if va.any() and not te_norm.any():
        print(f"    [{node}] ⚠ no normal windows for test. "
              f"Raise AMR_TEST_NORMAL_RATIO or collect more data.")

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
        print(f"✗ no data: {SRC}")
        print("  Set AMR_DATA to telemetry.json or its parent directory.")
        return 1

    print(f"Input: {SRC}")
    d = parse(SRC)
    if not len(d["X"]):
        print("✗ no valid windows")
        return 1

    moving = sch.moving_mask(d["X"])
    print(f"  windows {d['X'].shape}  (skipped {d['skipped']})")
    print(f"  moving {int(moving.sum())} / stopped {int((~moving).sum())}"
          f"  (speed threshold {sch.MIN_SPEED})")
    u, c = np.unique(d["label"], return_counts=True)
    print("  By state:", dict(zip(u.tolist(), c.tolist())))
    u, c = np.unique(d["robot"], return_counts=True)
    print("  By robot:", dict(zip(u.tolist(), c.tolist())))

    os.makedirs(OUT_DIR, exist_ok=True)
    rng = np.random.default_rng(SEED)

    # Count normal windows per site first and decide the cap.
    # With "min", match the smallest site.
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

        print(f"  Normal per site: "
              + " / ".join(f"{SITE_OF_NODE.get(n, n)} {c}"
                           for n, c in counts.items())
              + f"   (min/max {ratio:.2f})")

        if normal_cap > 0:
            if normal_cap < hi * MIN_BALANCE_RATIO:
                print(f"  ⚠ cap {normal_cap} is only "
                      f"{normal_cap / hi * 100:.0f}% of the largest site ({hi}).")
                print(f"    Balancing would discard most of the data. "
                      f"Better to keep it off until collection is even.")
                print(f"    To use it anyway, lower AMR_MIN_BALANCE_RATIO. "
                      f"Not applying the cap now.")
                normal_cap = 0
            else:
                print(f"  → capping at {normal_cap} "
                      f"(AMR_MAX_NORMAL_PER_SITE={MAX_NORMAL_PER_SITE})")
        else:
            print("  → no cap (to balance, AMR_MAX_NORMAL_PER_SITE=min)")
            if ratio < MIN_BALANCE_RATIO:
                print(f"    ⓘ sites differ a lot. FedAvg weights by sample count, "
                      f"so larger sites have more influence.")

    for node in NODES:
        pkl = build_node_pkl(d, node, rng, normal_cap)
        if not pkl:
            print(f"  ⚠ {node}({SITE_OF_NODE[node]}): no data assigned")
            continue
        want = EXPECTED_ROBOTS.get(node)
        n_rb = len(pkl["robots"])
        warn = "" if (want is None or n_rb == want) else f" ⚠expected {want}"
        path = os.path.join(OUT_DIR, f"{node}.pkl")
        with open(path, "wb") as fp:
            pickle.dump(pkl, fp)
        print(f"  ✓ {node}({pkl['site']}) {n_rb} robots{warn}: "
              f"train {len(pkl['train_signals'])} / val {len(pkl['val_signals'])} / "
              f"test {len(pkl['test_signals'])} (anomalies {int(pkl['test_labels'].sum())})")

    return 0


if __name__ == "__main__":
    sys.exit(main())
