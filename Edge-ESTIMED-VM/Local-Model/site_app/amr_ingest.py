"""
amr_ingest.py — fetches driving sensor data directly from contentInstances on the MN-CSE

[Why it is needed]
The previous implementation put only a local pkl path (data_path) in the con of cnt-sensor-data, and the MN-AE
read the file at that path directly. Exactly the same problem as with model weights.

  - It breaks immediately if the robot/simulator and the training app are on different machines
  - All oneM2M carries is a note saying 'the file is over there'

Here the actual signal in the CIN's con is read and turned into training input.
The con format is the same as what Isaac Sim uploads to the MN-CSE, and the same as one CIN in
the exported export/merged.json. So offline files and the live CSE use the same parser.

[con format]
    {"t_wall":…, "t_sim":[...], "nav":"…",
     "pose":{"x":[...],"y":[...],"yaw":[...]},
     "imu":{"front":{"ax":[...],…},"left":…,"right":…},
     "enc":{"drive":{"pos":[...],"vel":[...],"eff":[...]}, …},
     "robot":"siteA_AMR_1", "seq":12}

Channel layout and preprocessing are shared with prepare_data_amr (so only one place needs editing).

[Round separation]
Assumes Isaac Sim uploads CINs with a round_N label.
If there is no label, the round field inside con is used; if that is missing too, the latest N are used.
"""

from __future__ import annotations

import json
from typing import Dict, List, Optional, Tuple

import numpy as np

import amr_schema as sch
import config
import onem2m_utils as om2m


def parse_cin_window(payload: dict) -> Optional[Tuple[np.ndarray, dict]]:
    """
    One CIN con (dict) -> one window. None if channels are missing.
    Returns: (signal (C, T), meta)
    """
    sig = sch.window_from_con(payload)
    if sig is None:
        return None

    meta = {
        "robot": payload.get("robot", "unknown"),
        "seq": payload.get("seq"),
        "nav": payload.get("nav"),
        "t_wall": payload.get("t_wall"),
        "speed": sch.speed_of(sig),
        "pose": sch.pose_of(payload),
    }
    return sig, meta


def _labels_of(cin: dict) -> Dict[str, str]:
    lbl = (cin.get("m2m:cin") or {}).get("lbl") or []
    return {x.split(":")[0]: x.split(":", 1)[1] for x in lbl if ":" in x}


def _con_of(cin: dict) -> Optional[dict]:
    con = (cin.get("m2m:cin") or {}).get("con")
    try:
        obj = json.loads(con) if isinstance(con, str) else con
    except (TypeError, ValueError):
        return None
    return obj if isinstance(obj, dict) else None


def _empty() -> Dict[str, np.ndarray]:
    return {
        "signals": np.zeros((0, sch.N_CH, sch.T), dtype=np.float32),
        "cond": np.zeros((0, sch.COND_DIM), dtype=np.float32),
        "robot": np.asarray([]),
        "pose": np.zeros((0, 2), np.float32),
        "cond_name": np.asarray([]),
        "nav": np.asarray([]),
        "ct": np.asarray([]),
        # Time measured by the robot (epoch seconds). The sort key across the whole site.
        "t_wall": np.zeros(0, np.float64),
        # Per-robot sequence number. Used to find missing windows.
        "seq": np.zeros(0, np.int64),
        "max_ct": "",
        "n_raw": 0,
    }


def fetch_latest_windows(
    container_path: str,
    limit: int = 200,
) -> Dict[str, np.ndarray]:
    """
    Fetch the latest limit CINs regardless of round labels.

    This is the natural choice for real-time training. It corresponds to "the robots keep driving,
    and this round trains on the segments that just came in".
    """
    uril = om2m.list_uril(container_path, lim=limit)
    if not uril:
        return _empty()

    cins = []
    for u in uril[-limit:]:
        res = om2m.get_resource(u.lstrip("/"))
        if res and "m2m:cin" in res:
            cins.append(res)
    return build_from_cins(cins)


def balance_across_robots(got: Dict[str, np.ndarray],
                          cap: int) -> Dict[str, np.ndarray]:
    """
    Reduce the site's windows to at most cap, keeping them even across robots.

    If robot counts differ per site (A: 4, B: 2), giving each robot the same cap still
    leaves a 2x difference in site totals. FedAvg weights by sample count, so leaving it
    gives sites with more robots more influence. To balance per-site contributions
    it must be cut at the site level.

    Each robot is allotted cap / number of robots, and the share of robots with fewer windows is
    split among the rest. This keeps one robot from filling it all.
    Instead of cutting from the front, sample evenly across the whole time axis.
    """
    n = len(got["signals"])
    if cap <= 0 or n <= cap:
        return got

    robots = got["robot"]
    uniq = sorted(set(robots.tolist()))
    idx_by_robot = {r: np.where(robots == r)[0] for r in uniq}

    quota = {r: 0 for r in uniq}
    remaining = cap
    pool = list(uniq)
    while remaining > 0 and pool:
        share = max(1, remaining // len(pool))
        for r in list(pool):
            avail = len(idx_by_robot[r]) - quota[r]
            take = min(share, avail, remaining)
            quota[r] += take
            remaining -= take
            if quota[r] >= len(idx_by_robot[r]):
                pool.remove(r)
            if remaining <= 0:
                break

    keep = []
    for r in uniq:
        src = idx_by_robot[r]
        k = quota[r]
        if k <= 0:
            continue
        # Cutting only the front would see only the start of the run, so sample at even intervals.
        pick = np.linspace(0, len(src) - 1, k).astype(np.int64)
        keep.append(src[pick])

    keep = np.sort(np.concatenate(keep)) if keep else np.array([], dtype=np.int64)

    out = dict(got)
    for k in ("signals", "cond", "robot", "pose", "cond_name", "nav",
              "ct", "t_wall", "seq"):
        if k in out and len(out[k]) == n:
            out[k] = out[k][keep]
    return out


def fetch_new_windows(container_path: str,
                      since_ct: Optional[str] = None,
                      limit: int = 300) -> Dict[str, np.ndarray]:
    """
    Fetch only windows that arrived after since_ct.

    Scraping "the latest N" every round re-trains on nearly the same data when rounds are
    close together. Then you cannot tell whether improvement over rounds came from more data
    or from accumulated epochs. So the read position is remembered and only what comes after is read.

    Use the server's cra filter first; if it does not work, fetch and filter by ct.
    """
    uril = om2m.discover_cins_after(container_path, since_ct, lim=limit)
    if not uril:
        return _empty()

    # Take limit entries starting from the oldest.
    #
    # Previously it took 'the latest limit' with uril[-limit:]. uril comes oldest
    # first, so that skipped the earlier (older) segment. On top of that, the cursor
    # jumped to the max_ct of what was taken, so the skipped segment was never received.
    #
    # Measured: siteA's telemetry had 418 windows but mn1 took only 238.
    # 180 vanished this way, so at round 5 it could not reach 50
    # and stalled.
    #
    # Taking from the front makes the cursor advance a little at a time, and the rest stays
    # for the next query. Nothing is thrown away.
    #
    # (fetch_latest_windows is meant to get 'the latest N', so [-limit:] is right there.)
    cins = []
    for u in uril[:limit]:
        res = om2m.get_resource(u.lstrip("/"))
        if res and "m2m:cin" in res:
            cins.append(res)

    got = build_from_cins(cins)

    # For CSEs that ignore cra. Keep only what came after since_ct.
    if since_ct and len(got["signals"]):
        keep = got["ct"] > since_ct
        if not keep.all():
            for k in ("signals", "cond", "robot", "pose", "cond_name",
                      "nav", "ct", "t_wall", "seq"):
                got[k] = got[k][keep]
            got["max_ct"] = max([c for c in got["ct"].tolist() if c],
                                default=since_ct)
    return got


ARRAY_KEYS = ("signals", "cond", "robot", "pose",
              "cond_name", "nav", "ct", "t_wall", "seq")


def _reindex(got: Dict[str, np.ndarray], idx) -> Dict[str, np.ndarray]:
    """Rebuild several equal-length arrays at once with the same order/selection."""
    n = len(got["signals"])
    out = dict(got)
    for k in ARRAY_KEYS:
        if k in out and len(out[k]) == n:
            out[k] = out[k][idx]
    return out


def sort_by_time(got: Dict[str, np.ndarray]) -> Dict[str, np.ndarray]:
    """
    Line up the whole site in time order.

    [Why it is needed]
    fetch_site_windows_since fetches per robot and concatenates, so the buffer is
    grouped by robot like [all of AMR_1][all of AMR_2]. Cutting from the front as is
    would take everything from just one robot.

    [Why t_wall]
    ct is when it 'arrived' at the CSE. If one robot's publishing lags and then catches up in a burst,
    that robot's old data arrives after other robots' newest data.
    t_wall is when the robot 'measured' it, so the order holds even with lag.
    Falls back to ct for deployments without t_wall.
    """
    n = len(got["signals"])
    if n <= 1:
        return got

    tw = got.get("t_wall")
    if tw is not None and len(tw) == n and np.isfinite(tw).all():
        order = np.argsort(tw, kind="stable")
    else:
        order = np.argsort(got["ct"], kind="stable")

    return _reindex(got, order)


def take_head(got: Dict[str, np.ndarray], n: int):
    """
    Take the first n and return the rest.

    Nothing is discarded. The rest is used by the next round. So every round gets
    exactly n, and an anomaly from one moment is not scattered across rounds.

    Returns: (first n, rest)
    """
    total = len(got["signals"])
    if n <= 0:
        return _empty(), got
    if total <= n:
        return got, _empty()

    idx = np.arange(total)
    head = _reindex(got, idx[:n])
    tail = _reindex(got, idx[n:])

    head["max_ct"] = max([c for c in head["ct"].tolist() if c], default="")
    tail["max_ct"] = max([c for c in tail["ct"].tolist() if c], default="")
    head["n_raw"] = n
    tail["n_raw"] = total - n
    return head, tail


def robot_counts(got: Dict[str, np.ndarray]) -> Dict[str, int]:
    """Windows per robot. For seeing in the log whether one robot is hogging."""
    robots = got.get("robot")
    if robots is None or not len(robots):
        return {}
    uniq, cnt = np.unique(robots, return_counts=True)
    return {str(r): int(c) for r, c in zip(uniq, cnt)}


def seq_gaps(got: Dict[str, np.ndarray]) -> Dict[str, int]:
    """
    Count missing windows per robot.

    seq is a sequence number assigned by the robot, meaningful only within that robot. It should be
    contiguous; a jump means what lies between was lost. Until now windows could vanish
    silently with no way to know.
    """
    robots, seqs = got.get("robot"), got.get("seq")
    if robots is None or seqs is None or not len(seqs):
        return {}

    out = {}
    for r in sorted(set(robots.tolist())):
        v = np.sort(seqs[robots == r])
        v = v[v >= 0]
        if len(v) < 2:
            continue
        missing = int((np.diff(v) - 1).clip(min=0).sum())
        if missing:
            out[str(r)] = missing
    return out


def merge_windows(acc: Optional[Dict[str, np.ndarray]],
                  new: Dict[str, np.ndarray]) -> Dict[str, np.ndarray]:
    """
    Concatenate windows received over several fetches during a round.

    [Why it is needed]
    The robot telemetry container has an mni (max instances). It was created by the
    Isaac Sim side, so we do not set it. When a new CIN arrives, the oldest
    one is deleted.

    So "wait until 50 accumulate and then scrape them all at once" does not work.
    Re-querying with the same cursor misses whatever was deleted in between, so the total
    can never exceed robots × mni. That is why the 4-robot site got 57 and passed 50
    while the 2-robot sites froze at 31/33.

    Accumulating here on every fetch and advancing the cursor keeps what was grabbed
    before deletion in the buffer.
    """
    if acc is None or not len(acc.get("signals", [])):
        return new
    if new is None or not len(new.get("signals", [])):
        return acc

    out = {
        k: np.concatenate([acc[k], new[k]])
        for k in ("signals", "cond", "robot", "pose",
                  "cond_name", "nav", "ct", "t_wall", "seq")
    }
    out["max_ct"] = max(acc.get("max_ct", ""), new.get("max_ct", ""))
    out["n_raw"] = int(acc.get("n_raw", 0)) + int(new.get("n_raw", 0))
    return out


def fetch_site_windows_since(container_paths: List[str],
                             cursors: Dict[str, str],
                             limit_per_robot: int = 300) -> tuple:
    """
    Collect and merge only the windows after each robot's cursor across all robots at the site.

    Returns: (merged data, updated cursors)
    The cursor is the last ct per container path. Robots publish at different rates,
    so they are tracked separately.
    """
    parts, new_cursors = [], dict(cursors)

    for path in container_paths:
        got = fetch_new_windows(path, cursors.get(path), limit_per_robot)
        if len(got["signals"]):
            parts.append(got)
            if got["max_ct"]:
                new_cursors[path] = got["max_ct"]

    if not parts:
        return _empty(), new_cursors

    merged = {
        "signals": np.concatenate([p["signals"] for p in parts]),
        "cond": np.concatenate([p["cond"] for p in parts]),
        "robot": np.concatenate([p["robot"] for p in parts]),
        "pose": np.concatenate([p["pose"] for p in parts]),
        "cond_name": np.concatenate([p["cond_name"] for p in parts]),
        "nav": np.concatenate([p["nav"] for p in parts]),
        "ct": np.concatenate([p["ct"] for p in parts]),
        "t_wall": np.concatenate([p["t_wall"] for p in parts]),
        "seq": np.concatenate([p["seq"] for p in parts]),
        "max_ct": max([p["max_ct"] for p in parts], default=""),
        "n_raw": sum(p["n_raw"] for p in parts),
    }
    return merged, new_cursors


def fetch_site_windows(
    container_paths: List[str],
    limit_per_robot: int = 150,
) -> Dict[str, np.ndarray]:
    """
    Collect recent windows from several robots at one site and merge them into one.

    In the new data layout, telemetry accumulates under the robot AEs, not the MN-AE.
        <MN-CSE>/siteA_AMR_1/telemetry
        <MN-CSE>/siteA_AMR_2/telemetry   …
    One site is one FL node, so however many robots there are, they are merged here
    into "one factory's data". Exactly as designed.

    Even if one robot is dead, the round runs with the rest.
    """
    parts = []
    for path in container_paths:
        got = fetch_latest_windows(path, limit=limit_per_robot)
        if len(got["signals"]):
            parts.append(got)

    if not parts:
        return _empty()

    return {
        "signals": np.concatenate([p["signals"] for p in parts]),
        "cond": np.concatenate([p["cond"] for p in parts]),
        "robot": np.concatenate([p["robot"] for p in parts]),
        "pose": np.concatenate([p["pose"] for p in parts]),
        "cond_name": np.concatenate([p["cond_name"] for p in parts]),
        "nav": np.concatenate([p["nav"] for p in parts]),
        "ct": np.concatenate([p["ct"] for p in parts]),
        "max_ct": max([p["max_ct"] for p in parts], default=""),
        "n_raw": sum(p["n_raw"] for p in parts),
    }


def fetch_round_windows(
    container_path: str,
    round_num: int,
    limit: int = 400,
) -> Dict[str, np.ndarray]:
    """
    Gather one round's CINs into training input.

    container_path: usually "{MN-CSE}/{robot AE}/telemetry"
    Returned dict:
        signals (N, C, T) / cond (N, D) / robot / pose / cond_name / n_raw

    Search order
      1) CINs labeled round_N
      2) CINs whose round field in con matches
      3) If neither, the latest limit entries (treated as a live stream)
    """
    cins = om2m.retrieve_cins_by_label(
        container_path,
        f"round_{round_num}",
        lim=limit,
        fallback_scan=max(limit, 200),
        newest_first=False,          # Keep time order
    )

    if not cins:
        uril = om2m.list_uril(container_path, lim=max(limit, 200))
        for u in uril:
            res = om2m.get_resource(u.lstrip("/"))
            if not res:
                continue
            con = _con_of(res)
            if con is None:
                continue
            r = con.get("round", con.get("currentRound"))
            if r is not None and int(r) == int(round_num):
                cins.append(res)

    if not cins:
        # A deployment with no round separation at all. Train on the recent segment.
        print(f"    ⓘ round_{round_num} 표시가 없어 최근 {limit}개로 대체")
        return fetch_latest_windows(container_path, limit=limit)

    return build_from_cins(cins)


def build_from_cins(cins: List[Dict]) -> Dict[str, np.ndarray]:
    """List of CINs -> training input. Condition and state are read from labels."""
    sigs, conds, robots, poses, names, navs = [], [], [], [], [], []
    cts = []      # CIN creation time. Used as the cursor for the next round.
    twalls = []   # Time measured by the robot. Sorting uses this.
    seqs = []     # Per-robot sequence number. For checking losses.

    for cin in cins:
        con = _con_of(cin)
        if con is None:
            continue
        parsed = parse_cin_window(con)
        if parsed is None:
            continue
        sig, meta = parsed

        # The condition is derived from the payload. No labels are attached in operation.
        # If scn/spd labels exist, as in old data, they take priority.
        lab = _labels_of(cin)
        if "scn" in lab and "spd" in lab:
            cvec = sch.cond_from_labels(lab["scn"], lab["spd"])
        else:
            cvec = sch.cond_vector(meta["nav"], meta["speed"], sig)

        sigs.append(sig)
        conds.append(cvec)
        robots.append(meta["robot"])
        poses.append(meta["pose"])
        names.append(lab.get("cond", "floor_bump"
                             if meta["nav"] in ("over_bump",) else "normal"))
        navs.append(meta["nav"] or "idle")
        cts.append(str(cin.get("m2m:cin", {}).get("ct", "")))

        # NaN / -1 if absent. The sorting side falls back to ct.
        try:
            twalls.append(float(meta.get("t_wall")))
        except (TypeError, ValueError):
            twalls.append(float("nan"))
        try:
            seqs.append(int(meta.get("seq")))
        except (TypeError, ValueError):
            seqs.append(-1)

    if not sigs:
        out = _empty()
        out["n_raw"] = len(cins)
        return out

    return {
        "signals": np.stack(sigs).astype(np.float32),
        "cond": np.stack(conds).astype(np.float32),
        "robot": np.asarray(robots),
        "pose": np.asarray(poses, dtype=np.float32),
        "cond_name": np.asarray(names),
        "nav": np.asarray(navs),
        "ct": np.asarray(cts),
        "t_wall": np.asarray(twalls, dtype=np.float64),
        "seq": np.asarray(seqs, dtype=np.int64),
        "max_ct": max([c for c in cts if c], default=""),
        "n_raw": len(cins),
    }


def normalize_with(
    signals: np.ndarray,
    mean: Optional[np.ndarray],
    std: Optional[np.ndarray],
) -> np.ndarray:
    """
    Standardize with the normalization statistics kept by the node.

    The statistics must come from 'that site's normal training data'. Robots vary individually per site,
    so with shared statistics even normal data looks anomalous.
    If there are no statistics (first collection), compute them on the fly from the incoming data.
    """
    if signals.size == 0:
        return signals
    if mean is None or std is None:
        mean = signals.mean(axis=(0, 2), keepdims=True)
        std = signals.std(axis=(0, 2), keepdims=True) + 1e-8
    return ((signals - mean) / std).astype(np.float32)
