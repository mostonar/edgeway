"""
Edge-ESTIMED-VM/Local-Model/site_app/verdict.py — records anomaly verdicts on the site CSE.

[Where it is used]
Verdicts do not leave the site. The design lists "floor anomaly locations, robot inspection
status and driving instructions" as things to keep inside the site.
So they are kept on that site's MN-CSE, not the central IN-CSE.

  <MN-CSE>/ANALYSIS-AE-1/cnt-verdict/<cin>

[Why a separate AE rather than the MN-AE]
In the design the analysis app and the training app are different apps, and ACPs must be split
by role as well. For now the MN-AE, which holds the model, also computes verdicts, but they are written
with the analysis app identity (CANALYSIS-AE-i). If the analysis app is later split into a separate process,
the resource tree and the subscriber-side code stay the same.

[When to upload — not per window]
Upload only once, at the point where reconstruction error has exceeded the threshold 'K times in a row'
(config.ANOMALY_K_CONSECUTIVE, default 3). If it drops back to normal and then fills K again,
upload again at that point.

Otherwise CINs pile up for every window while the anomaly continues. At 500~600 windows
per factory, subscribers would receive the same fact hundreds of times.

[Subscriber side]
Putting a <subscription> (net=3) on cnt-verdict gives a notification for each new verdict.
m2m:sgn/nev/rep/m2m:cin/con in the notification body is the JSON string of the payload below.
lbl contains the robot name and verdict, so discovery for a specific robot is also possible.

  <cnt-verdict>?fu=1&ty=4&lbl=siteA_AMR_1
"""

import math
import os
import time
from typing import List, Optional, Sequence

import config
import onem2m_utils as om2m


# How many consecutive exceedances before a verdict is uploaded. Same value as offline analysis.
K_CONSECUTIVE = int(os.getenv(
    "FL_VERDICT_K", str(getattr(config, "ANOMALY_K_CONSECUTIVE", 3))))

# Whether to upload only once per robot. Once by default.
#
# If the same robot stays anomalous, CINs pile up every round. For the receiver, repeats after the
# first notice are not new information. So only the first one per robot is uploaded.
# Turning it off with FL_VERDICT_ONCE=0 uploads every time, as before.
ONCE_PER_ROBOT = os.getenv("FL_VERDICT_ONCE", "1") not in ("0", "false", "False")

# (node, robot) pairs already reported. Kept only while the process is alive.
# It is empty after a restart, so each robot goes out once more.
_reported: set = set()


def reset_reported() -> None:
    """Clear the report record. For testing."""
    _reported.clear()


def onset_indices(scores: Sequence[float], threshold: float,
                  k: Optional[int] = None) -> List[int]:
    """
    Indices where a run of k consecutive exceedances 'starts'.

    Not caught again while it continues. If it breaks and fills k again, it is caught again then.
    It fires at the k-th window, so the index points to that k-th one.
    """
    k = K_CONSECUTIVE if k is None else k
    hits: List[int] = []
    run = 0
    for i, s in enumerate(scores):
        if s > threshold:
            run += 1
            if run == k:
                hits.append(i)
        else:
            run = 0
    return hits


def _pose_at(poses, i) -> Optional[dict]:
    """
    Coordinates of window i. None if the value is unusable.

    pose_of returns NaN when coordinates are missing. Putting NaN in as is makes
    json.dumps emit the non-standard NaN token, which breaks the JSON parser on the
    subscriber side. So it is included only when finite.
    """
    if poses is None or i >= len(poses):
        return None

    try:
        x, y = float(poses[i][0]), float(poses[i][1])
    except (TypeError, ValueError, IndexError):
        return None

    if not (math.isfinite(x) and math.isfinite(y)):
        return None

    return {"x": round(x, 3), "y": round(y, 3), "basis": "window_mean"}


def publish(node_name: str,
            cnt_path: str,
            origin: str,
            scores,
            robots,
            threshold: float,
            round_num: int,
            cts=None,
            poses=None,
            model_uri: Optional[str] = None,
            k: Optional[int] = None,
            enrich=None) -> int:
    """
    Issue verdicts per robot and upload them as CINs. Returns the number uploaded.

    scores/robots/cts/poses are window-level arrays of the same length. Each robot is
    cut and looked at separately, so one robot's anomaly does not mix into another robot's consecutive count.

    With ONCE_PER_ROBOT (default true), only the first verdict per robot is uploaded. After that
    verdicts for the same robot are not uploaded even across rounds.

    poses are (x, y) of shape (N, 2). One window covers a stretch of driving, so it is
    the mean position of the samples in that stretch, not a coordinate pinned to one point. So the receiver
    does not misread it, basis="window_mean" is written into the CIN too.
    """
    if threshold is None or not len(scores):
        return 0

    k = K_CONSECUTIVE if k is None else k
    site = getattr(config, "SITE_OF_NODE", {}).get(node_name, node_name)
    total = 0

    for robot in sorted({str(r) for r in robots}):
        if ONCE_PER_ROBOT and (node_name, robot) in _reported:
            continue

        idx = [i for i, r in enumerate(robots) if str(r) == robot]
        if not idx:
            continue
        s_robot = [float(scores[i]) for i in idx]

        for hit in onset_indices(s_robot, threshold, k):
            i = idx[hit]
            score = float(scores[i])
            payload = {
                "type": "amr-verdict",
                "site": site,
                "node": node_name,
                "robot": robot,
                "verdict": "ANOMALY",
                "score": score,
                "threshold": float(threshold),
                "ratio": round(score / threshold, 3) if threshold else None,
                "consecutive": k,
                "window_index": i,
                "observed_at": (str(cts[i]) if cts is not None
                                and i < len(cts) else None),
                "pose": _pose_at(poses, i),
                "round": round_num,
                "model": model_uri,
                "timestamp": time.time(),
            }
            # MEC info (zone, AP, link). The verdict is uploaded as is even if this fails.
            if enrich is not None:
                try:
                    mec = enrich(robot)
                    if mec:
                        payload["mec"] = mec
                except Exception as e:
                    print(f"    ⚠ {robot}: failed to attach MEC info — {e}")
            res = om2m.create_content_instance(
                cnt_path, payload,
                labels=[robot, "ANOMALY", site],
                origin=origin,
            )
            if res:
                total += 1
                print(f"    ⚠ verdict {robot}: score={score:.6f} "
                      f"(threshold {threshold:.6f}, {k} in a row)")
                if ONCE_PER_ROBOT:
                    # First one per robot only. Later onsets in the same round are not uploaded either.
                    _reported.add((node_name, robot))
                    break

    if total:
        print(f"  ✓ {total} verdicts -> {om2m.strip_prefix(cnt_path)}")
    return total
