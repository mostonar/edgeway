"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_app.py — ETSI GS MEC 011 (Edge Platform Application Enablement) 클라이언트

사업장 학습 앱(TRAINING-AE-i)을 그 사업장 MEP 의 MEC 앱으로 등록하고,
MEP 가 제공하는 서비스(MEC013 위치, MEC028 WLAN)의 주소를 서비스
레지스트리에서 찾는다.

[순서]
  1) 앱 인스턴스 ID 찾기   샌드박스 컨트롤러 Applications API (MEC 명세 아님)
       GET  /{sbx}/sandbox-ctrl/v1/applications
     앱 인스턴스는 샌드박스 시나리오(edgeway)가 만든다. 여기서는 만들지도
     지우지도 않고, 이름(edgeway-fl-a)과 MEP(mep-a)로 찾기만 한다.
     없으면 시나리오가 예전 버전이라는 뜻이다. 등록을 건너뛰고 FL 은 계속한다.
  2) 준비 알림              MEC011 Application Support
       POST /{sbx}/{mep}/mec_app_support/v2/applications/{id}/confirm_ready
            {"indication": "READY"}
  3) 앱 등록                MEC011 Application Support
       POST /{sbx}/{mep}/mec_app_support/v2/registrations
            {"appName", "appInstanceId", "appProvider",
             "isInsByMec": false, "endpoint": {"uris": [...]}}
     MEP 밖에서 도는 앱이라 isInsByMec=false 이고, 그러면 endpoint 가 필수다.
     이 구현은 2) 보다 먼저 보내면 403 "Waiting for AppReadyConfirmation" 을
     돌려준다. 그래서 confirm_ready 다음에 보낸다.
  4) 서비스 발견            MEC011 Service Management
       GET  /{sbx}/{mep}/mec_service_mgmt/v1/services
  5) 종료                   등록만 지운다. 앱 인스턴스는 시나리오 것이라 둔다
       DELETE /{sbx}/{mep}/mec_app_support/v2/registrations/{id}

[실행 위치]
학습 앱은 MEP 안이 아니라 사업장 서버(지금은 연구실 서버)에서 돈다.
MEC 입장에서는 "MEP 에 등록된 외부 MEC 앱" 이다.

[주소 다시 쓰기]
레지스트리가 돌려주는 URI 는 샌드박스 내부 이름일 수 있다. 경로만 받아서
우리가 붙는 호스트(FL_SANDBOX_HOST) 위에 다시 붙인다.
"""

import os
from typing import Dict, List, Optional
from urllib.parse import urlparse

import requests

import config

TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "4"))


def _scheme() -> str:
    return os.getenv("FL_SANDBOX_SCHEME", "https")


def ctrl_base() -> str:
    override = os.getenv("FL_SANDBOX_CTRL_BASE")
    if override:
        return override.rstrip("/")
    return (f"{_scheme()}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/sandbox-ctrl/v1")


def mep_base(mep: str) -> str:
    return (f"{_scheme()}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/{mep}")


def _req(method: str, url: str, **kw):
    try:
        return requests.request(method, url, timeout=TIMEOUT,
                                verify=config.TLS_VERIFY,
                                headers={"Accept": "application/json",
                                         "Content-Type": "application/json"},
                                **kw)
    except Exception as e:
        print(f"  ✗ MEC011 {method} {url}: {e}")
        return None


# ── 1) 앱 인스턴스 ──────────────────────────────────────

def list_apps() -> List[dict]:
    r = _req("GET", f"{ctrl_base()}/applications")
    if r is None or r.status_code != 200:
        if r is not None:
            print(f"  ✗ sandbox-ctrl applications: HTTP {r.status_code}")
        return []
    try:
        data = r.json()
    except Exception:
        return []
    return data if isinstance(data, list) else []


def find_app(name: str, mep: str) -> Optional[str]:
    """시나리오가 만든 앱 인스턴스 ID. 같은 이름·같은 MEP 가 없으면 None."""
    for a in list_apps():
        if a.get("name") == name and a.get("nodeName") == mep and a.get("id"):
            return a["id"]
    print(f"  ✗ 시나리오에 FL 앱 {name}({mep}) 이 없다 — 샌드박스 시나리오를 확인할 것")
    return None


# ── 2) 준비 알림 ────────────────────────────────────────

def confirm_ready(mep: str, app_id: str) -> bool:
    """v2 를 먼저, 404 면 v1 로. 이미 READY 면 그대로 성공으로 본다."""
    for ver in ("v2", "v1"):
        url = (f"{mep_base(mep)}/mec_app_support/{ver}"
               f"/applications/{app_id}/confirm_ready")
        r = _req("POST", url, json={"indication": "READY"})
        if r is None:
            return False
        if r.status_code in (200, 204):
            return True
        if r.status_code != 404:
            print(f"  ✗ confirm_ready {ver}: HTTP {r.status_code} {r.text[:160]}")
            return False
    return False


# ── 3) 앱 등록 ──────────────────────────────────────────

APP_PROVIDER = os.getenv("FL_MEC_APP_PROVIDER", "edgeway")

# MEC 앱 이름. MEP 하나에 학습 앱 하나라서 MEP 이름에서 만든다
# (mep-a -> edgeway-fl-a). 시나리오의 앱 이름과 같아야 find_app 이 찾는다.
APP_PREFIX = os.getenv("FL_MEC_APP_PREFIX", "edgeway-fl")


def app_name(mep: str) -> str:
    return f"{APP_PREFIX}-{mep.split('-', 1)[-1]}"


def endpoint_for(port: int) -> str:
    """MEC011 등록에 넣는 앱 주소. FL_NOTIFY_HOST(oneM2M poa)와는 따로 둔다."""
    host = os.getenv("FL_MEC_APP_HOST") or config.NOTIFY_HOST
    return f"http://{host}:{port}"


def register(mep: str, app_id: str, name: str, endpoint: str) -> bool:
    """
    confirm_ready 다음에 부른다. 여러 번 불러도 결과가 같다.
      등록 없음            POST
      같은 endpoint 로 있음 그대로
      다른 endpoint 로 있음 PUT 으로 고친다
    """
    base = f"{mep_base(mep)}/mec_app_support/v2/registrations"
    body = {"appName": name, "appInstanceId": app_id,
            "appProvider": APP_PROVIDER,
            "appDescription": "Edgeway FL site trainer (AMR telemetry CVAE)",
            "isInsByMec": False,
            "endpoint": {"uris": [endpoint]}}
    r = _req("GET", f"{base}/{app_id}")
    if r is not None and r.status_code == 200:
        try:
            uris = ((r.json() or {}).get("endpoint") or {}).get("uris") or []
        except Exception:
            uris = []
        if endpoint in uris:
            return True
        r = _req("PUT", f"{base}/{app_id}", json=body)
        if r is not None and r.status_code in (200, 204):
            return True
        if r is not None:
            print(f"  ✗ registrations PUT: HTTP {r.status_code} {r.text[:160]}")
        return False
    r = _req("POST", base, json=body)
    if r is None:
        return False
    if r.status_code in (200, 201):
        return True
    print(f"  ✗ registrations: HTTP {r.status_code} {r.text[:160]}")
    return False


def deregister(mep: str, app_id: str) -> bool:
    r = _req("DELETE",
             f"{mep_base(mep)}/mec_app_support/v2/registrations/{app_id}")
    return r is not None and r.status_code in (200, 204, 404)


# ── 4) 서비스 발견 ──────────────────────────────────────

def services(mep: str) -> List[dict]:
    r = _req("GET", f"{mep_base(mep)}/mec_service_mgmt/v1/services")
    if r is None or r.status_code != 200:
        if r is not None:
            print(f"  ✗ 서비스 목록: HTTP {r.status_code}")
        return []
    try:
        data = r.json()
    except Exception:
        return []
    return data if isinstance(data, list) else []


def _uri_of(svc: dict) -> Optional[str]:
    ep = ((svc.get("transportInfo") or {}).get("endpoint") or {})
    uris = ep.get("uris") or []
    return uris[0] if uris else None


def _rebase(uri: str) -> str:
    """레지스트리 URI 의 경로만 남기고 우리가 붙는 호스트로 다시 붙인다."""
    path = urlparse(uri).path.rstrip("/")
    return f"{_scheme()}://{config.SANDBOX_HOST}{path}"


# 서비스 이름이 구현마다 달라서(meep-loc-serv, mec013-1 ...) 이름·분류·
# 경로에서 이 단어가 보이면 그 서비스로 본다.
KEYS = {
    "location": ("loc", "013", "location"),
    "wai": ("wai", "028", "wlan"),
}


def discover(mep: str) -> Dict[str, dict]:
    """{"location": {"name", "uri"}, "wai": {...}} 중 찾은 것만."""
    found: Dict[str, dict] = {}
    for s in services(mep):
        uri = _uri_of(s)
        if not uri or str(s.get("state", "ACTIVE")).upper() != "ACTIVE":
            continue
        hay = " ".join([str(s.get("serName", "")),
                        str((s.get("serCategory") or {}).get("name", "")),
                        urlparse(uri).path]).lower()
        for kind, words in KEYS.items():
            if kind not in found and any(w in hay for w in words):
                found[kind] = {"name": s.get("serName"), "uri": _rebase(uri),
                               "raw": uri}
    return found


def main() -> None:
    import argparse
    ap = argparse.ArgumentParser(description="MEC011 등록·서비스 발견 점검")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    ap.add_argument("--keep", action="store_true",
                    help="점검 뒤 등록을 지우지 않는다 (앱 인스턴스는 어느 쪽이든 그대로 둔다)")
    ap.add_argument("--endpoint-host", default=None,
                    help="MEC011 등록에 넣을 앱 주소 (기본 FL_MEC_APP_HOST, 없으면 FL_NOTIFY_HOST)")
    ap.add_argument("--base-port", type=int, default=5001,
                    help="mn1 의 포트. mn2, mn3 은 1씩 더한다")
    args = ap.parse_args()
    config.require_sandbox()
    print(f"sandbox-ctrl: {ctrl_base()}")
    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        name = app_name(mep)
        print(f"\n=== {node} / {mep} ===")
        app_id = find_app(name, mep)
        print(f"  앱 인스턴스: {app_id}")
        if app_id:
            print(f"  confirm_ready: {confirm_ready(mep, app_id)}")
            idx = config.ALL_NODES.index(node) if node in config.ALL_NODES else 0
            ep = (f"http://{args.endpoint_host}:{args.base_port + idx}"
                  if args.endpoint_host else endpoint_for(args.base_port + idx))
            print(f"  registrations: {register(mep, app_id, name, ep)}  ({ep})")
        for s in services(mep):
            print(f"  서비스 {s.get('serName')!s:20s} "
                  f"{(s.get('serCategory') or {}).get('name')!s:14s} "
                  f"{s.get('state')}  {_uri_of(s)}")
        print(f"  발견: {discover(mep)}")
        if app_id and not args.keep:
            print(f"  등록 삭제: {deregister(mep, app_id)}")


if __name__ == "__main__":
    main()
