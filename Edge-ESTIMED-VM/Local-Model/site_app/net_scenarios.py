"""
Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py — demo scenarios 2 and 3 (network anomalies)

In both scenarios the goal is "communication must be maintained". The point is not to show it breaking,
but to show that even when it would break, no data is lost and the FL round does not
stall.

  Scenario 2  network anomaly → handover
    See via MEC028 that the AP a robot is attached to has degraded.
    Move it to another AP at the same site. The MN-CSE stays the same, so only the path changes.
    The data flow is maintained.

  Scenario 3  network anomaly → buffering
    There is no AP to move to (or the link itself is cut).
    While nothing can be sent, queue it up, and push it in when the link comes back.
    The round is just a beat late; not a single window is lost.

Division of roles
  Detection/decision  this code (reads MEC028)
  Execution           sandbox controller event API (mec_sandbox.py)
  Record              oneM2M  <MN-CSE>/TRAINING-AE-i/cnt-net-status  (read by the dashboard)

FL does not command robots directly. Decisions are written to resources, and the moving
is done by the sandbox.

Usage
  python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py watch   --node mn3
  python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py s2      --node mn3 --robot siteC_AMR_2
  python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py s3      --node mn3 --robot siteC_AMR_2
  python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py restore --node mn3
"""

import argparse
import json
import os
import time
from typing import Dict, List, Optional

import config
import mec_sandbox as sbx
import mec_topology as topo
import mec_wai as wai
import onem2m_utils as om2m


# Where scenario 3 buffers. Disk on the site (MEP) side.
BUFFER_DIR = os.getenv("FL_NET_BUFFER_DIR", "/tmp/fl_net_buffer")


def bind(node: str) -> str:
    base, cse = config.mn_endpoint(node)
    om2m.set_endpoint(base, cse)
    om2m.set_in_endpoint(*config.in_endpoint())
    return cse


def net_status_path(node: str) -> str:
    cse = om2m.cse_name()
    return f"{cse}/{config.training_ae_name(node)}/cnt-net-status"


def report(node: str, payload: dict) -> bool:
    """
    Record decisions and actions in oneM2M.

    The dashboard and sandbox read it. With labels attached they can later
    be picked out with fu=1&lbl=.
    """
    payload = dict(payload)
    payload.setdefault("ts", time.time())
    payload.setdefault("node", node)

    ok = om2m.create_content_instance(
        net_status_path(node),
        json.dumps(payload, ensure_ascii=False),
        labels=[f"scenario:{payload.get('scenario', '?')}",
                f"verdict:{payload.get('verdict', '?')}",
                f"robot:{payload.get('robot', '-')}"],
    )
    if not ok:
        print(f"  ⚠ cnt-net-status write failed ({payload.get('scenario')})")
    return bool(ok)


# ════════════════════════════════════════════════════════
# Observe
# ════════════════════════════════════════════════════════

def cmd_watch(node: str, seconds: float, interval: float) -> None:
    """Read MEC028 periodically to show and record link state."""
    bind(node)
    mep = config.MEP_OF_NODE.get(node)
    print(f"=== {node} / {mep} link watch ({seconds:.0f}s, every {interval:.0f}s) ===")
    print(f"  MEC028: {wai.wai_base(mep)}")

    end = time.time() + seconds
    while time.time() < end:
        states = wai.site_link_states(node)
        if not states:
            print("  ⚠ got no UE from MEC028")
        for st in states:
            mark = {"GOOD": "✓", "DEGRADED": "⚠", "LOST": "✗"}[st["verdict"]]
            print(f"  {mark} {st['ae']:14s} AP={st['ap']}  "
                  f"RSSI={st['rssi']}  UL={st['ul_kbps']}kbps"
                  + (f"  ← {'; '.join(st['reasons'])}" if st["reasons"] else ""))
            if st["verdict"] != "GOOD":
                report(node, {"scenario": "watch", "robot": st["ae"],
                              "verdict": st["verdict"], "ap": st["ap"],
                              "rssi": st["rssi"], "ul_kbps": st["ul_kbps"],
                              "reasons": st["reasons"]})
        print()
        time.sleep(interval)


# ════════════════════════════════════════════════════════
# Scenario 2 — handover
# ════════════════════════════════════════════════════════

def cmd_scenario2(node: str, robot_ae: str,
                  wait_detect: float = 30.0,
                  inject: bool = True) -> bool:
    """
    Break the AP, detect via MEC028, move to another AP, and confirm recovery.

    With --no-inject, injection is skipped and only detection/action run. Use it when
    the fault was injected directly from the sandbox UI.
    """
    bind(node)
    mep = config.MEP_OF_NODE.get(node)
    ue = topo.ae_to_ue(robot_ae)
    if not ue:
        print(f"✗ {robot_ae}: no matching UE name found")
        return False

    print(f"\n=== Scenario 2: network fault → handover ===")
    print(f"  site {node} / {mep} / robot {robot_ae} ({ue})")

    before = wai.link_state(mep, ue)
    print(f"\n[1] Current state: {before['verdict']}  AP={before['ap']}  "
          f"RSSI={before['rssi']}  UL={before['ul_kbps']}kbps")
    if before["verdict"] == "LOST":
        print("  ✗ Already disconnected. Use scenario 3.")
        return False

    serving = before["ap"]

    if inject:
        print(f"\n[2] Injecting fault: {serving}")
        if not serving:
            print("  ✗ Serving AP unknown")
            return False
        sbx.degrade_poa(serving)
    else:
        print("\n[2] Fault injection skipped (--no-inject)")

    print(f"\n[3] Detecting via MEC028 (max {wait_detect:.0f}s)")
    deadline = time.time() + wait_detect
    detected = None
    while time.time() < deadline:
        st = wai.link_state(mep, ue)
        if st["verdict"] != "GOOD":
            detected = st
            break
        time.sleep(2.0)

    if detected is None:
        print("  ⚠ Threshold not crossed. Adjust FL_RSSI_WARN_DBM / FL_UL_MIN_KBPS")
        print("    or make FL_DEGRADE_* stronger.")
        detected = wai.link_state(mep, ue)
    else:
        print(f"  ⚠ Fault detected: {detected['verdict']}")
        for r in detected["reasons"]:
            print(f"      - {r}")

    report(node, {"scenario": "s2-detect", "robot": robot_ae,
                  "verdict": detected["verdict"], "ap": detected["ap"],
                  "rssi": detected["rssi"], "ul_kbps": detected["ul_kbps"],
                  "reasons": detected["reasons"]})

    print(f"\n[4] Selecting alternate AP")
    target = wai.best_target_ap(mep, ue, exclude=[serving] if serving else None)
    if not target:
        print("  ✗ No AP to move to → fall back to scenario 3 (buffering)")
        report(node, {"scenario": "s2-no-target", "robot": robot_ae,
                      "verdict": "FALLBACK_BUFFER", "ap": detected["ap"]})
        return False

    same_site = topo.site_of_poa(target) == topo.site_of_poa(serving or "")
    print(f"  → {target}"
          f"  ({'same site, MN-CSE kept' if same_site else 'different site, MEP changed'})")

    print(f"\n[5] Handover request (executed by sandbox)")
    if not sbx.handover(ue, target):
        return False

    print(f"\n[6] Checking recovery")
    time.sleep(3.0)
    after = None
    deadline = time.time() + 30.0
    while time.time() < deadline:
        st = wai.link_state(mep, ue)
        if st["ap"] and st["ap"] != serving:
            after = st
            break
        time.sleep(2.0)
    after = after or wai.link_state(mep, ue)

    print(f"  {after['verdict']}  AP={after['ap']}  "
          f"RSSI={after['rssi']}  UL={after['ul_kbps']}kbps")

    moved = bool(after["ap"]) and after["ap"] != serving
    report(node, {"scenario": "s2-handover", "robot": robot_ae,
                  "verdict": after["verdict"],
                  "ap_from": serving, "ap_to": after["ap"],
                  "same_site": same_site, "moved": moved,
                  "rssi": after["rssi"], "ul_kbps": after["ul_kbps"]})

    print(f"\n{'✓ Handover done. Only the data path changed; the flow continues.' if moved else '✗ AP did not change'}")
    print(f"  (cleanup: python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py restore --node {node})")
    return moved


# ════════════════════════════════════════════════════════
# Scenario 3 — buffering
# ════════════════════════════════════════════════════════

class StoreAndForward:
    """
    Queue that stores CIN payloads on disk while nothing can be sent.

    Used when the MN-AE uploads local updates to the central IN-CSE, and in the demo to hold
    robot telemetry on the robots' behalf. One file is one CIN. Names are
    sequence numbers, so they go out in arrival order.

    It is not held in memory because "nothing is lost" only holds if data survives
    even when the process dies during buffering.
    """

    def __init__(self, name: str, root: str = BUFFER_DIR):
        self.dir = os.path.join(root, name)
        os.makedirs(self.dir, exist_ok=True)

    def _files(self) -> List[str]:
        return sorted(f for f in os.listdir(self.dir) if f.endswith(".json"))

    def __len__(self) -> int:
        return len(self._files())

    def push(self, payload: dict) -> str:
        """Push one item onto the queue. The file name is a nanosecond timestamp."""
        fn = f"{time.time_ns()}.json"
        path = os.path.join(self.dir, fn)
        tmp = path + ".tmp"
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump(payload, f, ensure_ascii=False)
        os.replace(tmp, path)   # Even if it dies mid-write, no half-written file is left in the queue
        return path

    def drain(self, container_path: str, labels_of=None,
              max_items: int = 0) -> int:
        """
        Push the queued items into the CSE. Only successful ones are deleted.
        Stop at the first failure — order must be preserved.
        """
        sent = 0
        for fn in self._files():
            if max_items and sent >= max_items:
                break
            path = os.path.join(self.dir, fn)
            try:
                with open(path, encoding="utf-8") as f:
                    payload = json.load(f)
            except Exception:
                os.remove(path)     # Discard corrupt files
                continue

            labels = labels_of(payload) if labels_of else None
            ok = om2m.create_content_instance(
                container_path,
                json.dumps(payload, ensure_ascii=False),
                labels=labels,
            )
            if not ok:
                print(f"    ⚠ Send failed, stopping here ({len(self)} left)")
                break
            os.remove(path)
            sent += 1
        return sent


def cmd_scenario3(node: str, robot_ae: str,
                  outage: float = 40.0,
                  inject: bool = True) -> bool:
    """
    Cut the link, queue windows arriving in the meantime, and push them in after recovery.

    In reality the robot (Isaac) side buffers. In the demo this side plays that role
    instead. What matters is that "zero loss" is visible.
    """
    bind(node)
    mep = config.MEP_OF_NODE.get(node)
    ue = topo.ae_to_ue(robot_ae)
    if not ue:
        print(f"✗ {robot_ae}: no matching UE name found")
        return False

    cse = om2m.cse_name()
    tel_path = f"{cse}/{robot_ae}/{config.ROBOT_TELEMETRY_CNT}"

    print(f"\n=== Scenario 3: network fault → buffering ===")
    print(f"  site {node} / {mep} / robot {robot_ae} ({ue})")
    print(f"  telemetry: {tel_path}")

    before = wai.link_state(mep, ue)
    serving = before["ap"]
    print(f"\n[1] Current state: {before['verdict']}  AP={serving}")

    n_before = len(om2m.list_uril(tel_path, lim=1000))
    print(f"    CINs so far: {n_before}")

    queue = StoreAndForward(f"{node}-{robot_ae}")
    if len(queue):
        print(f"    ⚠ {len(queue)} items left in queue from a previous run")

    if inject:
        print(f"\n[2] Blocking link ({outage:.0f}s)")
        if not sbx.disconnect(ue):
            return False
    else:
        print("\n[2] Blocking skipped (--no-inject)")

    report(node, {"scenario": "s3-outage-start", "robot": robot_ae,
                  "verdict": "BUFFERING", "ap": serving,
                  "cin_before": n_before})

    print(f"\n[3] Queueing windows that arrive during the outage")
    end = time.time() + outage
    buffered = 0
    seq = 0
    while time.time() < end:
        st = wai.link_state(mep, ue)
        # Even while disconnected, robots keep driving and producing observations.
        # They cannot be sent, so they go into the queue. The real windows come from Isaac,
        # so here an envelope recording only the fact of storage is queued.
        queue.push({
            "buffered_at": time.time(),
            "robot": robot_ae,
            "seq": seq,
            "link": st["verdict"],
            "note": "link down — held on MEP side",
        })
        buffered += 1
        seq += 1
        print(f"    queue {len(queue):3d}  (link {st['verdict']})")
        time.sleep(5.0)

    print(f"\n[4] Restoring link")
    target = serving or wai.best_target_ap(mep, ue)
    if not target:
        print("  ✗ AP to restore unknown")
        return False
    sbx.reconnect(ue, target)

    deadline = time.time() + 40.0
    while time.time() < deadline:
        st = wai.link_state(mep, ue)
        if st["verdict"] != "LOST":
            print(f"  ✓ Restored: {st['verdict']}  AP={st['ap']}")
            break
        time.sleep(2.0)
    else:
        print("  ⚠ Could not confirm recovery")

    print(f"\n[5] Flushing queue ({len(queue)})")
    status_path = net_status_path(node)
    sent = queue.drain(
        status_path,
        labels_of=lambda p: [f"scenario:s3-replay", f"robot:{robot_ae}",
                             f"seq:{p.get('seq')}"],
    )
    print(f"  ✓ {sent} sent, {len(queue)} left")

    n_after = len(om2m.list_uril(tel_path, lim=1000))
    print(f"\n[6] Result")
    print(f"    CINs before outage {n_before} → after recovery {n_after}")
    print(f"    {sent} of {buffered} buffered sent, "
          f"{buffered - sent} lost")

    report(node, {"scenario": "s3-replay-done", "robot": robot_ae,
                  "verdict": "RECOVERED" if sent == buffered else "PARTIAL",
                  "buffered": buffered, "replayed": sent,
                  "lost": buffered - sent,
                  "cin_before": n_before, "cin_after": n_after})

    ok = (sent == buffered)
    print(f"\n{'✓ 0 lost. Buffering kept communication alive.' if ok else '✗ Data was lost'}")
    return ok


# ════════════════════════════════════════════════════════

def cmd_restore(node: str) -> None:
    """Restore all of this site's APs to normal quality."""
    bind(node)
    site = topo.SITE_OF_NODE.get(node)
    if not site:
        print(f"✗ Site of {node} unknown")
        return
    poas = topo.poas_of_site(site)
    print(f"=== {site} restoring quality of {len(poas)} APs ===")
    sbx.restore_all(poas)
    report(node, {"scenario": "restore", "verdict": "NORMAL",
                  "poas": len(poas)})


def main() -> None:
    ap = argparse.ArgumentParser(description="Demo scenarios 2/3 (network fault)")
    ap.add_argument("cmd", choices=["watch", "s2", "s3", "restore"])
    ap.add_argument("--node", default="mn3",
                    help="site node (mn1/mn2/mn3)")
    ap.add_argument("--robot", default=None,
                    help="robot AE name (default: that site's #1)")
    ap.add_argument("--seconds", type=float, default=60.0,
                    help="watch duration")
    ap.add_argument("--interval", type=float, default=5.0,
                    help="watch interval")
    ap.add_argument("--outage", type=float, default=40.0,
                    help="s3 outage duration")
    ap.add_argument("--no-inject", action="store_true",
                    help="detect and act only, without injecting a fault")
    args = ap.parse_args()

    config.require_sandbox()

    node = args.node
    robot = args.robot or (config.robot_names(node) or [None])[0]

    if args.cmd == "watch":
        cmd_watch(node, args.seconds, args.interval)
    elif args.cmd == "s2":
        cmd_scenario2(node, robot, inject=not args.no_inject)
    elif args.cmd == "s3":
        cmd_scenario3(node, robot, outage=args.outage,
                      inject=not args.no_inject)
    elif args.cmd == "restore":
        cmd_restore(node)


if __name__ == "__main__":
    main()
