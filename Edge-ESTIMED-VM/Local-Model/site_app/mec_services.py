"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_services.py — 사업장 학습 앱이 쓰는 MEC 서비스 묶음

  MEC011  학습 앱을 사업장 MEP 의 MEC 앱으로 등록하고(confirm_ready →
          registrations), MEC013·MEC028 주소를 서비스
          레지스트리에서 찾는다.                          FL_MEC_APP
  MEC028  라운드마다 로봇 링크 상태(AP·RSSI·전송률)를 읽어
          TRAINING-AE-i/cnt-net-status 에 남긴다.        FL_MEC_LINK
  MEC013  로봇이 있는 Zone·AP·좌표를 읽어 링크 기록과
          판정(cnt-verdict)에 같이 남긴다.               FL_MEC_LOCATION

  (MEC033 은 여기가 아니라 기동 맨 앞, MN-CSE 주소를 정할 때 쓴다.
   mec_iot.py, FL_MEC_IOT)

셋 다 "보조" 다. 실패해도 FL 라운드는 그대로 간다. 같은 서비스가 연달아
실패하면(FL_MEC_MAX_FAIL, 기본 3) 그 실행 동안은 더 부르지 않는다.
학습 시간을 MEC 응답 대기로 잃지 않기 위해서다.
"""

import atexit
import os
import time
from typing import Dict, List, Optional

import config
import mec_app
import mec_loc
import mec_topology as topo
import mec_wai


def _on(name: str, default: str = "1") -> bool:
    return os.getenv(name, default) not in ("0", "false", "False", "")


MAX_FAIL = int(os.getenv("FL_MEC_MAX_FAIL", "3"))


class _Breaker:
    """연속 실패가 MAX_FAIL 에 닿으면 끈다. 한 번 끄면 그 실행 동안 꺼져 있다."""

    def __init__(self, label: str, enabled: bool):
        self.label = label
        self.enabled = enabled
        self.fails = 0

    def ok(self) -> None:
        self.fails = 0

    def fail(self) -> None:
        self.fails += 1
        if self.enabled and self.fails >= MAX_FAIL:
            self.enabled = False
            print(f"  ⚠ {self.label}: {self.fails}번 연속 실패 — "
                  f"이번 실행에서는 더 부르지 않는다")


class SiteMec:
    def __init__(self, node: str, endpoint: Optional[str] = None):
        self.node = node
        # MEC011 등록에 넣는 앱 주소. 학습 앱의 알림 서버 주소를 쓴다.
        self.endpoint = endpoint
        self.mep = config.MEP_OF_NODE.get(node, "")
        self.app = _Breaker("MEC011", _on("FL_MEC_APP"))
        self.link = _Breaker("MEC028", _on("FL_MEC_LINK"))
        self.loc = _Breaker("MEC013", _on("FL_MEC_LOCATION"))
        self.app_id: Optional[str] = None
        self.ready = False
        self.registered = False
        self.discovered: Dict[str, dict] = {}

    @property
    def any_enabled(self) -> bool:
        return self.app.enabled or self.link.enabled or self.loc.enabled

    # ── 기동 ────────────────────────────────────────────
    def start(self) -> None:
        if not self.mep:
            return
        if not self.app.enabled:
            return
        print(f"\n  [{self.node}] MEC011 앱 등록 ({self.mep})")
        name = mec_app.app_name(self.mep)
        self.app_id = mec_app.find_app(name, self.mep)
        if not self.app_id:
            print(f"    ⚠ 앱 인스턴스가 없어 MEC011 등록을 건너뛴다. MEC013·028 은 기본 주소로 부른다")
            self.app.enabled = False
            return
        print(f"    ✓ 앱 인스턴스 {self.app_id}")
        # 정상 종료(Ctrl+C 포함) 때 등록만 지운다. 앱 인스턴스는 시나리오 것이다.
        # kill 로 끝나 등록이 남아도 다음 기동에서 그대로 확인·재사용한다.
        atexit.register(self.stop)
        self.ready = mec_app.confirm_ready(self.mep, self.app_id)
        print(f"    {'✓' if self.ready else '⚠'} confirm_ready "
              f"{'READY' if self.ready else '실패'}")
        # 등록은 READY 다음이어야 한다(먼저 보내면 403). 이전 실행의 등록이
        # 남아 있을 수 있어서 confirm_ready 결과와 상관없이 확인한다.
        if self.endpoint:
            self.registered = mec_app.register(
                self.mep, self.app_id, name, self.endpoint)
            print(f"    {'✓' if self.registered else '⚠'} registrations "
                  f"{self.endpoint if self.registered else '실패'}")
        else:
            print("    · registrations 건너뜀 (endpoint 없음)")

        self.discovered = mec_app.discover(self.mep)
        loc = self.discovered.get("location")
        wai = self.discovered.get("wai")
        if loc:
            mec_loc.DISCOVERED[self.mep] = loc["uri"]
        if wai:
            mec_wai.DISCOVERED[self.mep] = wai["uri"]
        for kind in ("location", "wai"):
            d = self.discovered.get(kind)
            print(f"    {'✓' if d else '·'} 서비스 발견 {kind}: "
                  f"{(d or {}).get('name')} {(d or {}).get('uri', '(기본 주소 사용)')}")

    def stop(self) -> None:
        if self.app_id and self.registered:
            ok = mec_app.deregister(self.mep, self.app_id)
            print(f"  [{self.node}] MEC011 등록 해제: {'✓' if ok else '⚠'}")
            self.registered = False

    # ── MEC013 ──────────────────────────────────────────
    def _location(self, ue: str) -> Optional[dict]:
        if not self.loc.enabled or not ue:
            return None
        return mec_loc.user_location(self.mep, ue)

    # ── 라운드마다 ──────────────────────────────────────
    def link_snapshot(self, round_num: int) -> Optional[dict]:
        """
        이 사업장 로봇들의 링크 상태(MEC028)와 위치(MEC013).
        cnt-net-status 에 그대로 올릴 수 있는 dict. 둘 다 꺼져 있으면 None.
        """
        if not (self.link.enabled or self.loc.enabled):
            return None
        robots: List[dict] = []
        if self.link.enabled:
            states = mec_wai.site_link_states(self.node)
            # 조회가 실패하면 로봇이 전부 'AP 없음(LOST)' 으로 나온다. 한 대라도
            # AP 가 보이면 MEC028 은 살아 있는 것으로 본다.
            (self.link.ok if any(r.get("ap") for r in states)
             else self.link.fail)()
            robots = states
        if not robots:
            robots = [{"ae": ae, "ue": topo.ae_to_ue(ae)}
                      for ae in config.robot_names(self.node)]
        located = 0
        for r in robots:
            loc = self._location(r.get("ue"))
            if loc:
                located += 1
                r["zone"] = loc.get("zone")
                r["loc_ap"] = loc.get("ap")
                r["lat"], r["lon"] = loc.get("lat"), loc.get("lon")
        if self.loc.enabled:
            # 로봇 한 대라도 위치가 나오면 MEC013 은 살아 있는 것으로 본다.
            (self.loc.ok if located else self.loc.fail)()

        summary: Dict[str, int] = {}
        for r in robots:
            v = r.get("verdict")
            if v:
                summary[v] = summary.get(v, 0) + 1
        return {
            "type": "net-status",
            "node": self.node,
            "mep": self.mep,
            "round": round_num,
            "timestamp": time.time(),
            "sources": [s for s, b in (("MEC028", self.link),
                                       ("MEC013", self.loc)) if b.enabled],
            "mec_app": {"appInstanceId": self.app_id, "ready": self.ready,
                        "registered": self.registered},
            "summary": summary,
            "robots": robots,
        }

    # ── 판정에 붙이기 ───────────────────────────────────
    def robot_context(self, robot_ae: str) -> Optional[dict]:
        """판정 한 건에 붙일 MEC 정보. 없으면 None."""
        ue = topo.ae_to_ue(robot_ae)
        if not ue:
            return None
        ctx: Dict[str, object] = {"ue": ue}
        loc = self._location(ue)
        if loc:
            ctx.update({"zone": loc.get("zone"), "ap": loc.get("ap"),
                        "lat": loc.get("lat"), "lon": loc.get("lon"),
                        "location_source": "MEC013"})
        if self.link.enabled:
            st = mec_wai.link_state(self.mep, ue)
            ctx.update({"link": st.get("verdict"), "rssi": st.get("rssi"),
                        "link_ap": st.get("ap"), "link_source": "MEC028"})
        return ctx if len(ctx) > 1 else None


def main() -> None:
    """학습 없이 MEC 연동만 점검한다: 등록 → 발견 → 링크·위치 한 번."""
    import argparse
    import json
    ap = argparse.ArgumentParser(description="사업장 MEC 연동 점검")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    args = ap.parse_args()
    config.require_sandbox()
    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        idx = config.ALL_NODES.index(node) if node in config.ALL_NODES else 0
        m = SiteMec(node, endpoint=mec_app.endpoint_for(5001 + idx))
        m.start()
        snap = m.link_snapshot(0)
        print(json.dumps(snap, ensure_ascii=False, indent=1)[:2000])
        robots = config.robot_names(node)
        if robots:
            print(f"  판정용 정보 {robots[0]}: {m.robot_context(robots[0])}")
        m.stop()


if __name__ == "__main__":
    main()
