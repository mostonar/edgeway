"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_services.py — bundle of MEC services used by the site training app

  MEC011  registers the training app as a MEC app of the site MEP (confirm_ready →
          registrations) and looks up MEC013/MEC028 addresses in the service
          registry.                                           FL_MEC_APP
  MEC028  every round, reads robot link state (AP, RSSI, data rate) and
          records it in TRAINING-AE-i/cnt-net-status.         FL_MEC_LINK
  MEC013  reads the zone, AP and coordinates of each robot and records them with link records
          and verdicts (cnt-verdict).                         FL_MEC_LOCATION

  (MEC033 is not used here but at the very start of startup, when deciding the MN-CSE address.
   mec_iot.py, FL_MEC_IOT)

All three are "auxiliary". The FL round goes on even if they fail. If the same service fails
repeatedly (FL_MEC_MAX_FAIL, default 3), it is not called again for the rest of the run.
This keeps training time from being lost waiting on MEC responses.
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
    """Turn it off when consecutive failures reach MAX_FAIL. Once off, it stays off for the run."""

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
        # App address put into the MEC011 registration. Uses the training app's notification server address.
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

    # ── Startup ─────────────────────────────────────────
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
        # On normal shutdown (including Ctrl+C) only the registration is deleted. The app instance belongs to the scenario.
        # If it ended with kill and the registration remains, the next startup checks and reuses it as is.
        atexit.register(self.stop)
        self.ready = mec_app.confirm_ready(self.mep, self.app_id)
        print(f"    {'✓' if self.ready else '⚠'} confirm_ready "
              f"{'READY' if self.ready else '실패'}")
        # Registration must come after READY (sending it first gives 403). A registration from a previous run
        # may remain, so it is checked regardless of the confirm_ready result.
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

    # ── Every round ─────────────────────────────────────
    def link_snapshot(self, round_num: int) -> Optional[dict]:
        """
        Link state (MEC028) and location (MEC013) of this site's robots.
        A dict that can be uploaded to cnt-net-status as is. None if both are off.
        """
        if not (self.link.enabled or self.loc.enabled):
            return None
        robots: List[dict] = []
        if self.link.enabled:
            states = mec_wai.site_link_states(self.node)
            # If the query fails, every robot shows up as 'no AP (LOST)'. If even one robot
            # sees an AP, MEC028 is considered alive.
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
            # If even one robot's location comes back, MEC013 is considered alive.
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

    # ── Attach to verdicts ──────────────────────────────
    def robot_context(self, robot_ae: str) -> Optional[dict]:
        """MEC info to attach to one verdict. None if there is none."""
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
    """Check only the MEC integration without training: register → discover → link and location once."""
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
