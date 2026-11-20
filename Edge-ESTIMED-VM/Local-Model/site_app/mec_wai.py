"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_wai.py — ETSI GS MEC 028 (WLAN Information API, WAI) client

Each MEP in the MEC sandbox runs mec028-1 (meep-wais). Which AP a robot is attached to
and what its signal and data rate are come from here. We do not guess.

Endpoints (MEP-local service)
  GET /{sbx}/{mep}/wai/v2/queries/ap/ap_information    AP state
  GET /{sbx}/{mep}/wai/v2/queries/sta/sta_information  terminal state
  POST /{sbx}/{mep}/wai/v2/subscriptions               AssocSta / StaDataRate

Fields used (MEC028 v2.3.1)
  StaInfo.apAssociated.bssid       AP currently attached to
  StaInfo.rssi.rssi                received signal (dBm, negative)
  StaInfo.staDataRate.staLastDataUplinkRate    uplink data rate (kbps)
  ApInfo.bssLoad.channelUtilization            channel utilization (0~255 = 0~100%)
  ApInfo.bssLoad.staCount                      number of attached terminals

This file is not the side that 'creates' network faults. That is done by the sandbox controller's
event API (net_scenarios.py). This file only observes.
"""

import os
from typing import Dict, List, Optional

import requests

import config
import mec_topology as topo


# channelUtilization from MEC028 uses 255 as 100% (IEEE 802.11 BSS Load).
CHU_FULL = 255.0

# Decision thresholds. Set against the sandbox's default Wi-Fi model; change via environment variables.
RSSI_WARN = float(os.getenv("FL_RSSI_WARN_DBM", "-75"))   # At or below: warning
RSSI_BAD = float(os.getenv("FL_RSSI_BAD_DBM", "-85"))     # At or below: bad
CHU_WARN = float(os.getenv("FL_CHU_WARN_PCT", "70"))      # Channel utilization %
UL_MIN_KBPS = float(os.getenv("FL_UL_MIN_KBPS", "500"))   # Minimum uplink data rate

TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "8"))

# If an address was found via MEC011 service discovery, use it. mep -> base url
DISCOVERED: Dict[str, str] = {}


def _verify():
    return config.TLS_VERIFY


def wai_base(mep: str) -> str:
    """
    Base URL of the MEP-local WAI service.
    The AdvantEDGE ingress rewrites /{sbx}/{mep}/wai to /wai.
    """
    override = os.getenv(f"FL_WAI_BASE_{mep.replace('-', '_').upper()}")
    if override:
        return override.rstrip("/")
    if mep in DISCOVERED:
        return DISCOVERED[mep]
    scheme = os.getenv("FL_SANDBOX_SCHEME", "https")
    return (f"{scheme}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/{mep}/wai/v2")


def _get(mep: str, path: str) -> Optional[dict]:
    url = f"{wai_base(mep)}{path}"
    try:
        r = requests.get(url, timeout=TIMEOUT, verify=_verify(),
                         headers={"Accept": "application/json"})
    except Exception as e:
        print(f"  ✗ MEC028 {mep}{path}: {e}")
        return None
    if r.status_code != 200:
        print(f"  ✗ MEC028 {mep}{path}: HTTP {r.status_code}")
        return None
    try:
        return r.json()
    except Exception:
        print(f"  ✗ MEC028 {mep}{path}: JSON 파싱 실패")
        return None


# ── Queries ─────────────────────────────────────────────

def ap_information(mep: str) -> List[dict]:
    """List of APs this MEP sees. ApInfo array."""
    data = _get(mep, "/queries/ap/ap_information")
    if not data:
        return []
    # Depending on the implementation it comes as {"apInfo": [...]} or as a bare array.
    if isinstance(data, list):
        return data
    for k in ("apInfo", "ApInfo", "apInfoList"):
        if isinstance(data.get(k), list):
            return data[k]
    return []


def sta_information(mep: str) -> List[dict]:
    """List of terminals this MEP sees. StaInfo array."""
    data = _get(mep, "/queries/sta/sta_information")
    if not data:
        return []
    if isinstance(data, list):
        return data
    for k in ("staInfo", "StaInfo", "staInfoList"):
        if isinstance(data.get(k), list):
            return data[k]
    return []


# ── Interpretation ──────────────────────────────────────

def ap_load_by_mac(mep: str) -> Dict[str, dict]:
    """
    BSSID -> {"name", "channel_util_pct", "sta_count"}
    Convert channelUtilization to a percentage. Using the 255 scale as is
    makes thresholds unreadable.
    """
    out: Dict[str, dict] = {}
    for ap in ap_information(mep):
        bssid = ((ap.get("apId") or {}).get("bssid") or "").lower()
        if not bssid:
            continue
        load = ap.get("bssLoad") or {}
        chu = load.get("channelUtilization")
        out[bssid] = {
            "name": topo.poa_name_of_mac(bssid) or bssid,
            "channel_util_pct": (float(chu) / CHU_FULL * 100.0
                                 if chu is not None else None),
            "sta_count": load.get("staCount"),
            "channel": ap.get("channel"),
        }
    return out


def station(mep: str, ue_name: str,
            stas: Optional[List[dict]] = None) -> Optional[dict]:
    """
    Find one StaInfo by UE name (robot-c2).
    MEC028 refers only by MAC, so convert using the topology table.

    If stas is given, use it. Avoids querying every time when looking at several robots.
    """
    mac = topo.UE_MAC.get(ue_name, "").lower()
    if not mac:
        return None
    for sta in (stas if stas is not None else sta_information(mep)):
        if ((sta.get("staId") or {}).get("macId") or "").lower() == mac:
            return sta
    return None


def link_state(mep: str, ue_name: str,
               stas: Optional[List[dict]] = None,
               loads: Optional[Dict[str, dict]] = None) -> dict:
    """
    Summarize one robot's link state at a glance.

    Returns
      verdict  "GOOD" | "DEGRADED" | "LOST"
      reasons  sentences explaining the decision (so they can be shown as is on the demo screen)
      ap       name of the AP currently attached to
      rssi, ul_kbps, dl_kbps, channel_util_pct

    LOST means there is no AP association or MEC028 cannot see the terminal at all.
    DEGRADED means attached, but signal, data rate or channel utilization crossed a threshold.
    """
    sta = station(mep, ue_name, stas=stas)
    if sta is None:
        return {"ue": ue_name, "verdict": "LOST", "ap": None,
                "rssi": None, "ul_kbps": None, "dl_kbps": None,
                "channel_util_pct": None,
                "reasons": ["MEC028에서 단말이 보이지 않는다"]}

    assoc = sta.get("apAssociated") or {}
    bssid = (assoc.get("bssid") or "").lower()
    ap_name = topo.poa_name_of_mac(bssid) or bssid or None

    rssi = (sta.get("rssi") or {}).get("rssi")
    rate = sta.get("staDataRate") or {}
    ul = rate.get("staLastDataUplinkRate")
    dl = rate.get("staLastDataDownlinkRate")

    chu = None
    if bssid:
        if loads is None:
            loads = ap_load_by_mac(mep)
        chu = (loads.get(bssid) or {}).get("channel_util_pct")

    reasons: List[str] = []
    if not bssid:
        reasons.append("붙어 있는 AP가 없다")
    if rssi is not None and float(rssi) <= RSSI_BAD:
        reasons.append(f"신호 {rssi}dBm (불량 기준 {RSSI_BAD:.0f})")
    elif rssi is not None and float(rssi) <= RSSI_WARN:
        reasons.append(f"신호 {rssi}dBm (주의 기준 {RSSI_WARN:.0f})")
    if ul is not None and float(ul) < UL_MIN_KBPS:
        reasons.append(f"상향 {ul}kbps (최소 {UL_MIN_KBPS:.0f})")
    if chu is not None and chu >= CHU_WARN:
        reasons.append(f"채널 점유 {chu:.0f}% (기준 {CHU_WARN:.0f}%)")

    # No attached AP means LOST; crossing even one threshold means DEGRADED.
    # A signal below the bad threshold is already added to reasons above.
    if not bssid:
        verdict = "LOST"
    elif reasons:
        verdict = "DEGRADED"
    else:
        verdict = "GOOD"

    return {"ue": ue_name, "verdict": verdict, "ap": ap_name,
            "rssi": rssi, "ul_kbps": ul, "dl_kbps": dl,
            "channel_util_pct": chu, "reasons": reasons}


def site_link_states(node: str) -> List[dict]:
    """Link state of all robots at one site."""
    mep = config.MEP_OF_NODE.get(node)
    if not mep:
        return []
    # Querying per robot means one round trip per robot. Fetch once and reuse.
    stas = sta_information(mep)
    loads = ap_load_by_mac(mep)

    out = []
    for ae in config.robot_names(node):
        ue = topo.ae_to_ue(ae)
        if not ue:
            continue
        st = link_state(mep, ue, stas=stas, loads=loads)
        st["ae"] = ae
        out.append(st)
    return out


def best_target_ap(mep: str, ue_name: str,
                   exclude: Optional[List[str]] = None) -> Optional[str]:
    """
    Pick the handover target AP.

    First priority is the neighbor AP report (neighborReport) seen by the terminal itself. It is evidence
    the terminal can actually hear it, so it beats guessing.
    Second priority is the AP at the same site with the lowest channel utilization.
    Third priority is a spare-site AP (the MEP changes, so last resort).
    """
    skip = {s.lower() for s in (exclude or [])}
    skip |= {(topo.POA_MAC.get(s, "") or "").lower() for s in (exclude or [])}

    loads = ap_load_by_mac(mep)
    sta = station(mep, ue_name)

    # 1) Neighbor report
    if sta:
        cur = ((sta.get("apAssociated") or {}).get("bssid") or "").lower()
        neigh = []
        for nb in sta.get("neighborReport") or []:
            bssid = (nb.get("bssid") or "").lower()
            if not bssid or bssid == cur or bssid in skip:
                continue
            name = topo.poa_name_of_mac(bssid)
            if not name or name in skip:
                continue
            chu = (loads.get(bssid) or {}).get("channel_util_pct")
            neigh.append((chu if chu is not None else 50.0, name))
        if neigh:
            neigh.sort()
            return neigh[0][1]

    # 2) 3) Topology candidates with lower channel utilization
    cands = topo.handover_candidates(ue_name, exclude=list(skip))
    scored = []
    for name in cands:
        mac = (topo.POA_MAC.get(name) or "").lower()
        chu = (loads.get(mac) or {}).get("channel_util_pct")
        # APs not visible from this MEP (= spare) are pushed back but kept as candidates.
        scored.append((0 if mac in loads else 1,
                       chu if chu is not None else 50.0, name))
    if not scored:
        return None
    scored.sort()
    return scored[0][2]


# ── Standalone run: dump current link state ─────────────

def main() -> None:
    import argparse
    ap = argparse.ArgumentParser(
        description="MEC028으로 로봇 링크 상태를 본다")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    args = ap.parse_args()

    config.require_sandbox()

    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        site = config.SITE_OF_NODE.get(node, node)
        print(f"\n=== {node} / {site} / {mep} ===")
        print(f"  {wai_base(mep)}")

        aps = ap_load_by_mac(mep)
        if aps:
            print(f"  AP {len(aps)}개")
            for mac, info in aps.items():
                chu = info["channel_util_pct"]
                print(f"    {info['name']:22s} ch{info['channel']}  "
                      f"점유 {chu:.0f}%  단말 {info['sta_count']}대"
                      if chu is not None else
                      f"    {info['name']:22s} ch{info['channel']}")
        else:
            print("  ⚠ AP 정보를 못 받았다 (mec028-1이 떠 있는지 확인)")

        for st in site_link_states(node):
            mark = {"GOOD": "✓", "DEGRADED": "⚠", "LOST": "✗"}[st["verdict"]]
            print(f"    {mark} {st['ae']:14s} ({st['ue']:9s}) "
                  f"AP={st['ap']}  RSSI={st['rssi']}  UL={st['ul_kbps']}kbps")
            for r in st["reasons"]:
                print(f"        - {r}")


if __name__ == "__main__":
    main()
