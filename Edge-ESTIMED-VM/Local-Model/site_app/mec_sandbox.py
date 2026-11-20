"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_sandbox.py — MEC sandbox controller event client

The side issuing commands is the sandbox, not FL. FL/MN-AE only detect and decide;
actually moving robots or breaking links is done by this API.

This API is the control API of AdvantEDGE (MEC sandbox) itself, not an ETSI MEC
spec. So it has no GS MEC number. The standard APIs are in mec_app.py (MEC011),
mec_loc.py (MEC013) and mec_wai.py (MEC028).

  POST /{sbx}/sandbox-ctrl/v1/events/MOBILITY
       {"name": "...", "type": "MOBILITY",
        "eventMobility": {"elementName": "robot-c2",
                          "dest": "site-c-poa-wifi-5"}}       AP switch
       Setting dest to "DISCONNECTED" cuts the link.

  POST /{sbx}/sandbox-ctrl/v1/events/NETWORK-CHARACTERISTICS-UPDATE
       {"name": "...", "type": "NETWORK-CHARACTERISTICS-UPDATE",
        "eventNetworkCharacteristicsUpdate": {
            "elementName": "site-c-poa-wifi-2", "elementType": "POA",
            "netChar": {"latency": 300, "latencyVariation": 100,
                        "throughputDl": 1, "throughputUl": 1,
                        "packetLoss": 40}}}                    quality degradation

Order used in the demo
  1) Inject fault    degrade_poa()  or disconnect()
  2) Detect         mec_wai.link_state()  — observed via MEC028
  3) Act            handover()  or buffering
  4) Recover        restore_poa() / reconnect()

elementType can be POA, UE, EDGE, FOG, CLOUD, POA-CELL, etc. A Wi-Fi AP is a POA.
"""

import os
import time
from typing import Dict, List, Optional

import requests

import config


TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "8"))

# Normal quality of a site AP. Same as the POA-WIFI netChar in edgeway.yaml.
# Restored to these values on recovery.
POA_NETCHAR_NORMAL: Dict[str, int] = {
    "latency": 1,
    "latencyVariation": 1,
    "throughputDl": 1000,
    "throughputUl": 1000,
    "packetLoss": 0,
}

# For scenario 2. Attached but unusable.
POA_NETCHAR_DEGRADED: Dict[str, int] = {
    "latency": int(os.getenv("FL_DEGRADE_LATENCY_MS", "250")),
    "latencyVariation": int(os.getenv("FL_DEGRADE_JITTER_MS", "120")),
    "throughputDl": int(os.getenv("FL_DEGRADE_DL_MBPS", "2")),
    "throughputUl": int(os.getenv("FL_DEGRADE_UL_MBPS", "1")),
    "packetLoss": int(os.getenv("FL_DEGRADE_LOSS_PCT", "35")),
}


def ctrl_base() -> str:
    override = os.getenv("FL_SANDBOX_CTRL_BASE")
    if override:
        return override.rstrip("/")
    scheme = os.getenv("FL_SANDBOX_SCHEME", "https")
    return (f"{scheme}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/sandbox-ctrl/v1")


def _post_event(event_type: str, body: dict) -> bool:
    url = f"{ctrl_base()}/events/{event_type}"
    try:
        r = requests.post(url, json=body, timeout=TIMEOUT,
                          verify=config.TLS_VERIFY,
                          headers={"Content-Type": "application/json"})
    except Exception as e:
        print(f"  ✗ 샌드박스 이벤트 {event_type}: {e}")
        return False
    if r.status_code not in (200, 201, 204):
        print(f"  ✗ 샌드박스 이벤트 {event_type}: "
              f"HTTP {r.status_code} {r.text[:160]}")
        return False
    return True


# ── MOBILITY ────────────────────────────────────────────

def handover(ue_name: str, dest_poa: str) -> bool:
    """Attach a robot to another AP. dest_poa is the element name (site-c-poa-wifi-5)."""
    ok = _post_event("MOBILITY", {
        "name": f"handover-{ue_name}-{int(time.time())}",
        "type": "MOBILITY",
        "eventMobility": {"elementName": ue_name, "dest": dest_poa},
    })
    print(f"  {'✓' if ok else '✗'} 핸드오버 {ue_name} → {dest_poa}")
    return ok


def disconnect(ue_name: str) -> bool:
    """Cut the robot's wireless link. Starting point of scenario 3 (buffering)."""
    ok = _post_event("MOBILITY", {
        "name": f"disconnect-{ue_name}-{int(time.time())}",
        "type": "MOBILITY",
        "eventMobility": {"elementName": ue_name, "dest": "DISCONNECTED"},
    })
    print(f"  {'✓' if ok else '✗'} 링크 차단 {ue_name}")
    return ok


def reconnect(ue_name: str, poa: str) -> bool:
    """Reattach a disconnected robot."""
    return handover(ue_name, poa)


# ── NETWORK-CHARACTERISTICS-UPDATE ──────────────────────

def set_poa_netchar(poa_name: str, netchar: Dict[str, int]) -> bool:
    ok = _post_event("NETWORK-CHARACTERISTICS-UPDATE", {
        "name": f"netchar-{poa_name}-{int(time.time())}",
        "type": "NETWORK-CHARACTERISTICS-UPDATE",
        "eventNetworkCharacteristicsUpdate": {
            "elementName": poa_name,
            "elementType": "POA",
            "netChar": dict(netchar),
        },
    })
    return ok


def degrade_poa(poa_name: str,
                netchar: Optional[Dict[str, int]] = None) -> bool:
    """Degrade AP quality. Starting point of scenario 2."""
    nc = netchar or POA_NETCHAR_DEGRADED
    ok = set_poa_netchar(poa_name, nc)
    print(f"  {'✓' if ok else '✗'} AP 품질 저하 {poa_name}  "
          f"지연 {nc['latency']}ms, 손실 {nc['packetLoss']}%, "
          f"상향 {nc['throughputUl']}Mbps")
    return ok


def restore_poa(poa_name: str) -> bool:
    """Restore AP quality to normal."""
    ok = set_poa_netchar(poa_name, POA_NETCHAR_NORMAL)
    print(f"  {'✓' if ok else '✗'} AP 품질 복구 {poa_name}")
    return ok


def restore_all(poa_names: List[str]) -> None:
    for p in poa_names:
        restore_poa(p)
        time.sleep(0.2)
