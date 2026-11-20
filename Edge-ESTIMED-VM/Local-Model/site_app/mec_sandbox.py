"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_sandbox.py — MEC 샌드박스 컨트롤러 이벤트 클라이언트

명령을 내리는 쪽은 FL이 아니라 샌드박스다. FL/MN-AE는 탐지하고 판정만 하고,
실제로 로봇을 옮기거나 링크를 망가뜨리는 것은 이 API가 한다.

이 API는 AdvantEDGE(MEC 샌드박스) 자체의 제어 API이고 ETSI MEC 명세가
아니다. 그래서 GS MEC 번호가 없다. 표준 API는 mec_app.py(MEC011),
mec_loc.py(MEC013), mec_wai.py(MEC028) 쪽이다.

  POST /{sbx}/sandbox-ctrl/v1/events/MOBILITY
       {"name": "...", "type": "MOBILITY",
        "eventMobility": {"elementName": "robot-c2",
                          "dest": "site-c-poa-wifi-5"}}       AP 전환
       dest를 "DISCONNECTED"로 주면 링크를 끊는다.

  POST /{sbx}/sandbox-ctrl/v1/events/NETWORK-CHARACTERISTICS-UPDATE
       {"name": "...", "type": "NETWORK-CHARACTERISTICS-UPDATE",
        "eventNetworkCharacteristicsUpdate": {
            "elementName": "site-c-poa-wifi-2", "elementType": "POA",
            "netChar": {"latency": 300, "latencyVariation": 100,
                        "throughputDl": 1, "throughputUl": 1,
                        "packetLoss": 40}}}                    품질 저하

데모에서 쓰는 순서
  1) 장애 주입    degrade_poa()  또는 disconnect()
  2) 탐지         mec_wai.link_state()  — MEC028으로 본다
  3) 조치         handover()  또는 버퍼링
  4) 복구         restore_poa() / reconnect()

elementType은 POA, UE, EDGE, FOG, CLOUD, POA-CELL 등이 있다. Wi-Fi AP는 POA다.
"""

import os
import time
from typing import Dict, List, Optional

import requests

import config


TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "8"))

# 사업장 AP의 평상시 품질. edgeway.yaml의 POA-WIFI netChar 그대로다.
# 복구할 때 이 값으로 되돌린다.
POA_NETCHAR_NORMAL: Dict[str, int] = {
    "latency": 1,
    "latencyVariation": 1,
    "throughputDl": 1000,
    "throughputUl": 1000,
    "packetLoss": 0,
}

# 시나리오 2용. 붙어는 있지만 쓸 수 없는 수준.
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
    """로봇을 다른 AP에 붙인다. dest_poa는 요소 이름(site-c-poa-wifi-5)."""
    ok = _post_event("MOBILITY", {
        "name": f"handover-{ue_name}-{int(time.time())}",
        "type": "MOBILITY",
        "eventMobility": {"elementName": ue_name, "dest": dest_poa},
    })
    print(f"  {'✓' if ok else '✗'} 핸드오버 {ue_name} → {dest_poa}")
    return ok


def disconnect(ue_name: str) -> bool:
    """로봇의 무선 링크를 끊는다. 시나리오 3(버퍼링)의 출발점."""
    ok = _post_event("MOBILITY", {
        "name": f"disconnect-{ue_name}-{int(time.time())}",
        "type": "MOBILITY",
        "eventMobility": {"elementName": ue_name, "dest": "DISCONNECTED"},
    })
    print(f"  {'✓' if ok else '✗'} 링크 차단 {ue_name}")
    return ok


def reconnect(ue_name: str, poa: str) -> bool:
    """끊었던 로봇을 다시 붙인다."""
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
    """AP 품질을 떨어뜨린다. 시나리오 2의 출발점."""
    nc = netchar or POA_NETCHAR_DEGRADED
    ok = set_poa_netchar(poa_name, nc)
    print(f"  {'✓' if ok else '✗'} AP 품질 저하 {poa_name}  "
          f"지연 {nc['latency']}ms, 손실 {nc['packetLoss']}%, "
          f"상향 {nc['throughputUl']}Mbps")
    return ok


def restore_poa(poa_name: str) -> bool:
    """AP 품질을 평상시로 되돌린다."""
    ok = set_poa_netchar(poa_name, POA_NETCHAR_NORMAL)
    print(f"  {'✓' if ok else '✗'} AP 품질 복구 {poa_name}")
    return ok


def restore_all(poa_names: List[str]) -> None:
    for p in poa_names:
        restore_poa(p)
        time.sleep(0.2)
