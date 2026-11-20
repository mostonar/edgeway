"""
Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py — 데모 시나리오 2·3 (네트워크 이상)

두 시나리오 모두 "통신은 유지돼야 한다"가 목표다. 끊기는 모습을 보이는 게
아니라, 끊길 상황에서도 데이터가 안 사라지고 FL 라운드가 안 멈추는 모습을
보이는 것이다.

  시나리오 2  네트워크 이상 → 핸드오버
    MEC028로 로봇이 붙어 있는 AP가 나빠진 것을 본다.
    같은 사업장의 다른 AP로 옮긴다. MN-CSE가 그대로이므로 경로만 바뀐다.
    데이터 흐름은 유지된다.

  시나리오 3  네트워크 이상 → 버퍼링
    옮겨갈 AP가 없다(또는 링크 자체가 끊겼다).
    보낼 수 없는 동안은 큐에 쌓아 두고, 링크가 돌아오면 밀어 넣는다.
    라운드가 한 템포 늦어질 뿐 윈도우는 하나도 안 잃는다.

역할 분담
  탐지·판정  이 코드 (MEC028을 읽는다)
  실행       샌드박스 컨트롤러 이벤트 API (mec_sandbox.py)
  기록       oneM2M  <MN-CSE>/TRAINING-AE-i/cnt-net-status  (대시보드가 읽는다)

FL이 로봇에게 직접 명령하지 않는다. 판정은 리소스에 써 두고, 움직이는 일은
샌드박스가 한다.

사용법
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


# 시나리오 3에서 버퍼에 쌓아 두는 위치. 사업장(MEP) 쪽 디스크다.
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
    판정·조치 기록을 oneM2M에 남긴다.

    대시보드와 샌드박스가 이걸 읽는다. 라벨을 붙여 두면 나중에
    fu=1&lbl=로 골라 올 수 있다.
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
        print(f"  ⚠ cnt-net-status 기록 실패 ({payload.get('scenario')})")
    return bool(ok)


# ════════════════════════════════════════════════════════
# 관찰
# ════════════════════════════════════════════════════════

def cmd_watch(node: str, seconds: float, interval: float) -> None:
    """MEC028을 주기적으로 읽어 링크 상태를 보여주고 기록한다."""
    bind(node)
    mep = config.MEP_OF_NODE.get(node)
    print(f"=== {node} / {mep} 링크 감시 ({seconds:.0f}s, {interval:.0f}s 간격) ===")
    print(f"  MEC028: {wai.wai_base(mep)}")

    end = time.time() + seconds
    while time.time() < end:
        states = wai.site_link_states(node)
        if not states:
            print("  ⚠ MEC028에서 단말을 못 받았다")
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
# 시나리오 2 — 핸드오버
# ════════════════════════════════════════════════════════

def cmd_scenario2(node: str, robot_ae: str,
                  wait_detect: float = 30.0,
                  inject: bool = True) -> bool:
    """
    AP를 망가뜨리고, MEC028로 탐지하고, 다른 AP로 옮기고, 회복을 확인한다.

    --no-inject를 주면 주입은 건너뛰고 탐지·조치만 한다. 샌드박스 UI에서
    직접 장애를 넣었을 때 쓴다.
    """
    bind(node)
    mep = config.MEP_OF_NODE.get(node)
    ue = topo.ae_to_ue(robot_ae)
    if not ue:
        print(f"✗ {robot_ae}에 대응하는 UE 이름을 못 찾았다")
        return False

    print(f"\n=== 시나리오 2: 네트워크 이상 → 핸드오버 ===")
    print(f"  사업장 {node} / {mep} / 로봇 {robot_ae} ({ue})")

    before = wai.link_state(mep, ue)
    print(f"\n[1] 현재 상태: {before['verdict']}  AP={before['ap']}  "
          f"RSSI={before['rssi']}  UL={before['ul_kbps']}kbps")
    if before["verdict"] == "LOST":
        print("  ✗ 지금 이미 끊겨 있다. 시나리오 3을 써라.")
        return False

    serving = before["ap"]

    if inject:
        print(f"\n[2] 장애 주입: {serving}")
        if not serving:
            print("  ✗ 붙어 있는 AP를 모르겠다")
            return False
        sbx.degrade_poa(serving)
    else:
        print("\n[2] 장애 주입 생략 (--no-inject)")

    print(f"\n[3] MEC028으로 탐지 (최대 {wait_detect:.0f}s)")
    deadline = time.time() + wait_detect
    detected = None
    while time.time() < deadline:
        st = wai.link_state(mep, ue)
        if st["verdict"] != "GOOD":
            detected = st
            break
        time.sleep(2.0)

    if detected is None:
        print("  ⚠ 문턱을 넘지 않았다. FL_RSSI_WARN_DBM / FL_UL_MIN_KBPS를")
        print("    조정하거나 FL_DEGRADE_* 를 더 세게 줘라.")
        detected = wai.link_state(mep, ue)
    else:
        print(f"  ⚠ 이상 감지: {detected['verdict']}")
        for r in detected["reasons"]:
            print(f"      - {r}")

    report(node, {"scenario": "s2-detect", "robot": robot_ae,
                  "verdict": detected["verdict"], "ap": detected["ap"],
                  "rssi": detected["rssi"], "ul_kbps": detected["ul_kbps"],
                  "reasons": detected["reasons"]})

    print(f"\n[4] 대체 AP 선정")
    target = wai.best_target_ap(mep, ue, exclude=[serving] if serving else None)
    if not target:
        print("  ✗ 옮겨갈 AP가 없다 → 시나리오 3(버퍼링)으로 넘어가야 한다")
        report(node, {"scenario": "s2-no-target", "robot": robot_ae,
                      "verdict": "FALLBACK_BUFFER", "ap": detected["ap"]})
        return False

    same_site = topo.site_of_poa(target) == topo.site_of_poa(serving or "")
    print(f"  → {target}"
          f"  ({'같은 사업장, MN-CSE 유지' if same_site else '다른 사업장, MEP 변경'})")

    print(f"\n[5] 핸드오버 요청 (샌드박스가 실행한다)")
    if not sbx.handover(ue, target):
        return False

    print(f"\n[6] 회복 확인")
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

    print(f"\n{'✓ 핸드오버 완료. 데이터 경로가 바뀌었을 뿐 흐름은 유지된다.' if moved else '✗ AP가 바뀌지 않았다'}")
    print(f"  (정리는: python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py restore --node {node})")
    return moved


# ════════════════════════════════════════════════════════
# 시나리오 3 — 버퍼링
# ════════════════════════════════════════════════════════

class StoreAndForward:
    """
    보낼 수 없는 동안 CIN 페이로드를 디스크에 쌓아 두는 큐.

    MN-AE가 중앙 IN-CSE로 로컬 업데이트를 올릴 때, 그리고 데모에서 로봇
    텔레메트리를 대신 보관할 때 쓴다. 파일 하나가 CIN 하나다. 이름이
    순번이라 들어온 순서대로 나간다.

    메모리에 들고 있지 않는 이유는, 버퍼링 중에 프로세스가 죽어도
    데이터가 남아야 "하나도 안 잃는다"가 성립하기 때문이다.
    """

    def __init__(self, name: str, root: str = BUFFER_DIR):
        self.dir = os.path.join(root, name)
        os.makedirs(self.dir, exist_ok=True)

    def _files(self) -> List[str]:
        return sorted(f for f in os.listdir(self.dir) if f.endswith(".json"))

    def __len__(self) -> int:
        return len(self._files())

    def push(self, payload: dict) -> str:
        """큐에 하나 쌓는다. 파일명은 나노초 타임스탬프."""
        fn = f"{time.time_ns()}.json"
        path = os.path.join(self.dir, fn)
        tmp = path + ".tmp"
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump(payload, f, ensure_ascii=False)
        os.replace(tmp, path)   # 쓰다 죽어도 반쪽 파일이 큐에 안 남는다
        return path

    def drain(self, container_path: str, labels_of=None,
              max_items: int = 0) -> int:
        """
        쌓인 것을 CSE로 밀어 넣는다. 성공한 것만 지운다.
        하나라도 실패하면 거기서 멈춘다 — 순서를 지켜야 한다.
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
                os.remove(path)     # 깨진 파일은 버린다
                continue

            labels = labels_of(payload) if labels_of else None
            ok = om2m.create_content_instance(
                container_path,
                json.dumps(payload, ensure_ascii=False),
                labels=labels,
            )
            if not ok:
                print(f"    ⚠ 전송 실패, 여기서 멈춘다 (남은 {len(self)}개)")
                break
            os.remove(path)
            sent += 1
        return sent


def cmd_scenario3(node: str, robot_ae: str,
                  outage: float = 40.0,
                  inject: bool = True) -> bool:
    """
    링크를 끊고, 그 동안 들어오는 윈도우를 큐에 쌓고, 복구 후 밀어 넣는다.

    현실에서는 로봇(Isaac) 쪽이 버퍼링한다. 데모에서는 그 역할을 이 쪽에서
    대신 보여준다. 중요한 것은 "유실 0"이 눈에 보이는 것이다.
    """
    bind(node)
    mep = config.MEP_OF_NODE.get(node)
    ue = topo.ae_to_ue(robot_ae)
    if not ue:
        print(f"✗ {robot_ae}에 대응하는 UE 이름을 못 찾았다")
        return False

    cse = om2m.cse_name()
    tel_path = f"{cse}/{robot_ae}/{config.ROBOT_TELEMETRY_CNT}"

    print(f"\n=== 시나리오 3: 네트워크 이상 → 버퍼링 ===")
    print(f"  사업장 {node} / {mep} / 로봇 {robot_ae} ({ue})")
    print(f"  텔레메트리: {tel_path}")

    before = wai.link_state(mep, ue)
    serving = before["ap"]
    print(f"\n[1] 현재 상태: {before['verdict']}  AP={serving}")

    n_before = len(om2m.list_uril(tel_path, lim=1000))
    print(f"    지금까지 쌓인 CIN {n_before}개")

    queue = StoreAndForward(f"{node}-{robot_ae}")
    if len(queue):
        print(f"    ⚠ 이전 실행의 큐가 {len(queue)}개 남아 있다")

    if inject:
        print(f"\n[2] 링크 차단 ({outage:.0f}s)")
        if not sbx.disconnect(ue):
            return False
    else:
        print("\n[2] 차단 생략 (--no-inject)")

    report(node, {"scenario": "s3-outage-start", "robot": robot_ae,
                  "verdict": "BUFFERING", "ap": serving,
                  "cin_before": n_before})

    print(f"\n[3] 차단 동안 들어오는 윈도우를 큐에 쌓는다")
    end = time.time() + outage
    buffered = 0
    seq = 0
    while time.time() < end:
        st = wai.link_state(mep, ue)
        # 끊긴 동안에도 로봇은 계속 주행하고 관측을 만든다.
        # 보낼 수 없으니 큐에 넣는다. 실제 윈도우는 Isaac이 주므로
        # 여기서는 보관 사실만 기록하는 봉투를 넣는다.
        queue.push({
            "buffered_at": time.time(),
            "robot": robot_ae,
            "seq": seq,
            "link": st["verdict"],
            "note": "link down — held on MEP side",
        })
        buffered += 1
        seq += 1
        print(f"    큐 {len(queue):3d}개  (링크 {st['verdict']})")
        time.sleep(5.0)

    print(f"\n[4] 링크 복구")
    target = serving or wai.best_target_ap(mep, ue)
    if not target:
        print("  ✗ 되돌릴 AP를 모르겠다")
        return False
    sbx.reconnect(ue, target)

    deadline = time.time() + 40.0
    while time.time() < deadline:
        st = wai.link_state(mep, ue)
        if st["verdict"] != "LOST":
            print(f"  ✓ 복구됨: {st['verdict']}  AP={st['ap']}")
            break
        time.sleep(2.0)
    else:
        print("  ⚠ 복구를 확인하지 못했다")

    print(f"\n[5] 큐 비우기 ({len(queue)}개)")
    status_path = net_status_path(node)
    sent = queue.drain(
        status_path,
        labels_of=lambda p: [f"scenario:s3-replay", f"robot:{robot_ae}",
                             f"seq:{p.get('seq')}"],
    )
    print(f"  ✓ {sent}개 전송, 남은 {len(queue)}개")

    n_after = len(om2m.list_uril(tel_path, lim=1000))
    print(f"\n[6] 결과")
    print(f"    차단 전 CIN {n_before}개 → 복구 후 {n_after}개")
    print(f"    버퍼에 쌓았던 {buffered}개 중 {sent}개 전송, "
          f"유실 {buffered - sent}개")

    report(node, {"scenario": "s3-replay-done", "robot": robot_ae,
                  "verdict": "RECOVERED" if sent == buffered else "PARTIAL",
                  "buffered": buffered, "replayed": sent,
                  "lost": buffered - sent,
                  "cin_before": n_before, "cin_after": n_after})

    ok = (sent == buffered)
    print(f"\n{'✓ 유실 0. 버퍼링으로 통신이 유지됐다.' if ok else '✗ 유실이 있다'}")
    return ok


# ════════════════════════════════════════════════════════

def cmd_restore(node: str) -> None:
    """이 사업장 AP 전부를 평상시 품질로 되돌린다."""
    bind(node)
    site = topo.SITE_OF_NODE.get(node)
    if not site:
        print(f"✗ {node}의 사업장을 모르겠다")
        return
    poas = topo.poas_of_site(site)
    print(f"=== {site} AP {len(poas)}개 품질 복구 ===")
    sbx.restore_all(poas)
    report(node, {"scenario": "restore", "verdict": "NORMAL",
                  "poas": len(poas)})


def main() -> None:
    ap = argparse.ArgumentParser(description="데모 시나리오 2·3 (네트워크 이상)")
    ap.add_argument("cmd", choices=["watch", "s2", "s3", "restore"])
    ap.add_argument("--node", default="mn3",
                    help="사업장 노드 (mn1/mn2/mn3)")
    ap.add_argument("--robot", default=None,
                    help="로봇 AE 이름 (기본: 그 사업장 1번)")
    ap.add_argument("--seconds", type=float, default=60.0,
                    help="watch 지속 시간")
    ap.add_argument("--interval", type=float, default=5.0,
                    help="watch 간격")
    ap.add_argument("--outage", type=float, default=40.0,
                    help="s3 차단 시간")
    ap.add_argument("--no-inject", action="store_true",
                    help="장애를 주입하지 않고 탐지·조치만 한다")
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
