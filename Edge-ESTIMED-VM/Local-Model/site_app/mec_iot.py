"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_iot.py — ETSI GS MEC 033 (IoT API) 클라이언트

사업장 앱이 자기 MN-CSE 주소를 MEC 에게 묻는다. MEP 의 IoT 서비스(meep-iot)에
등록된 IoT 플랫폼이 바로 그 사업장의 MN-CSE 다.

  GET /{sbx}/{mep}/iots/v1/registered_iot_platforms
      [{"iotPlatformId", "enabled",
        "userTransportInfo":          [{"name", "protocol", "endpoint": {...}}],
        "customServicesTransportInfo":[{"name", "protocol", "endpoint": {...}}]}]

  transport 의 name 이 CSE 이름이고 endpoint 의 uris 가 CSE 주소다
  (meep-sss 도 같은 필드로 CSE 를 찾는다).

[쓰는 순서]
  1) FL_MN_BASE_URL_<NODE> 를 직접 줬으면 그걸 쓴다. 사람이 정한 값이 이긴다.
  2) MEC033 에서 받은 주소를 실제로 CSEBase 조회해 본다. 응답하면 그걸 쓴다.
  3) 아니면 기존 규칙(config.mn_endpoint)으로 돌아간다. FL 은 멈추지 않는다.

[주소 다시 쓰기]
등록된 URI 는 샌드박스 내부 이름일 수 있다. MEC011 서비스 발견과 같이 경로만
받아서 우리가 붙는 호스트(FL_SANDBOX_HOST) 위에 다시 붙인다. 그래서 2)의
확인이 필요하다. 경로가 내부 전용이면 조회가 실패하고 3)으로 간다.

끄기: FL_MEC_IOT=0
"""

import os
import uuid
from typing import List, Optional, Tuple
from urllib.parse import urlparse

import requests

import config
import onem2m_utils as om2m

TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "4"))


def _on() -> bool:
    return os.getenv("FL_MEC_IOT", "1") not in ("0", "false", "False", "")


def _scheme() -> str:
    return os.getenv("FL_SANDBOX_SCHEME", "https")


def iot_base(mep: str) -> str:
    override = os.getenv(f"FL_IOT_BASE_{mep.replace('-', '_').upper()}")
    if override:
        return override.rstrip("/")
    return (f"{_scheme()}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/{mep}/iots/v1")


def platforms(mep: str) -> List[dict]:
    url = f"{iot_base(mep)}/registered_iot_platforms"
    try:
        r = requests.get(url, timeout=TIMEOUT, verify=config.TLS_VERIFY,
                         headers={"Accept": "application/json"})
    except Exception as e:
        print(f"  ✗ MEC033 {mep}: {e}")
        return []
    if r.status_code != 200:
        print(f"  ✗ MEC033 {mep}: HTTP {r.status_code}")
        return []
    try:
        data = r.json()
    except Exception:
        return []
    if isinstance(data, dict):
        data = [data]
    return data if isinstance(data, list) else []


def _uri_of(tr: dict) -> Optional[str]:
    ep = tr.get("endpoint") or {}
    uris = ep.get("uris") or []
    if uris:
        return uris[0]
    addrs = ep.get("addresses") or []
    if addrs and addrs[0].get("host"):
        a = addrs[0]
        port = f":{a['port']}" if a.get("port") else ""
        return f"http://{a['host']}{port}"
    return None


def _rebase(uri: str) -> Optional[str]:
    path = urlparse(uri).path.rstrip("/")
    if not path:
        return None          # 호스트만 있는 내부 주소는 밖에서 못 쓴다
    return f"{_scheme()}://{config.SANDBOX_HOST}{path}"


def candidates(mep: str) -> List[Tuple[str, str, str]]:
    """[(base_url, cse_name, iotPlatformId)] — 이 MEP 에 등록된 HTTP CSE 들."""
    out = []
    for p in platforms(mep):
        if p.get("enabled") is False:
            continue
        trs = (p.get("customServicesTransportInfo") or []) + \
              (p.get("userTransportInfo") or [])
        for tr in trs:
            proto = str(tr.get("protocol") or "").upper()
            if proto and "HTTP" not in proto:
                continue      # MQTT 등은 우리가 쓰지 않는다
            uri, cse = _uri_of(tr), tr.get("name")
            base = _rebase(uri) if uri else None
            if base and cse:
                out.append((base, cse, str(p.get("iotPlatformId") or "")))
    # 플랫폼이 여럿이면 CSE 이름에 MEP 이름이 든 것을 앞에 둔다.
    out.sort(key=lambda c: mep not in c[1])
    return out


def _get_cse_base(base: str, cse: str, origin: Optional[str] = None):
    """CSEBase 를 한 번 RETRIEVE 한다. origin 을 안 주면 지금 신원으로 나간다."""
    try:
        return requests.get(
            f"{base}/{cse}", timeout=TIMEOUT, verify=config.TLS_VERIFY,
            auth=config.mn_auth(),
            headers={"Accept": "application/json",
                     "X-M2M-Origin": origin or om2m.originator(),
                     "X-M2M-RI": f"mec033-{uuid.uuid4().hex[:8]}",
                     "X-M2M-RVI": "3"})
    except Exception:
        return None


def probe(base: str, cse: str) -> Optional[int]:
    """CSEBase 조회의 HTTP 상태. 연결 실패·타임아웃이면 None."""
    r = _get_cse_base(base, cse)
    return None if r is None else r.status_code


def cse_answers(base: str, cse: str, origin: Optional[str] = None) -> bool:
    """정말 CSE 인지까지 본다(200 이고 본문이 m2m:cb)."""
    r = _get_cse_base(base, cse, origin)
    return r is not None and r.status_code == 200 and "m2m:cb" in r.text


_cse_answers = cse_answers   # 예전 이름


def resolve(node: str) -> Tuple[str, str]:
    """사업장 노드의 (base_url, cse_name). MEC033 → 확인 → 실패 시 기존 규칙."""
    fallback = config.mn_endpoint(node)
    mep = config.MEP_OF_NODE.get(node, "")
    if os.getenv(f"FL_MN_BASE_URL_{node.upper()}"):
        print(f"  [{node}] MN-CSE: FL_MN_BASE_URL_{node.upper()} 사용 (MEC033 건너뜀)")
        return fallback
    if not _on() or not mep or not config.sandbox_ready():
        return fallback
    for base, cse, pid in candidates(mep):
        if _cse_answers(base, cse):
            same = (base, cse) == fallback
            print(f"  [{node}] MN-CSE: MEC033 {mep} 플랫폼 {pid or '?'} → "
                  f"{base}/{cse}  {'(규칙 주소와 같음)' if same else '(규칙 주소와 다름)'}")
            return base, cse
        print(f"  [{node}] MEC033 후보 {base}/{cse} 응답 없음")
    print(f"  [{node}] MN-CSE: MEC033 에서 못 받아 규칙 주소 사용 "
          f"{fallback[0]}/{fallback[1]}")
    return fallback


def main() -> None:
    """점검: python3 Edge-ESTIMED-VM/Local-Model/site_app/mec_iot.py [mn1,mn2,mn3]"""
    import sys
    config.require_sandbox()
    nodes = (sys.argv[1] if len(sys.argv) > 1 else ",".join(config.ALL_NODES))
    for node in [n.strip() for n in nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        print(f"\n=== {node} / {mep} ===")
        for p in platforms(mep):
            print(f"  플랫폼 {p.get('iotPlatformId')} enabled={p.get('enabled')}")
        for c in candidates(mep):
            print(f"  후보 {c[0]}/{c[1]}")
        print(f"  결과 {resolve(node)}")


if __name__ == "__main__":
    main()
