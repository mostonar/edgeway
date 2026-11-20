"""
oneM2M Utility Functions (ACME CSE, Mca/Mcc)
- AE / CNT / CIN CRUD
- ACP (Access Control Policy) 생성
- Subscription
- Discovery with filterCriteria-like query (label-based)
- Retry: timeout/connection 에러 시 지수 백오프 재시도
"""

from __future__ import annotations

import itertools
import json
import os
import time
import requests
from typing import Dict, List, Optional, Any
import config


# -------------------------
# Retry 설정
# -------------------------
MAX_RETRY     = 4
RETRY_DELAY   = 3.0
RETRY_BACKOFF = 1.5

# ─────────────────────────────────────────────────────────
# 엔드포인트
#
# 사업장마다 MN-CSE가 따로다(mep-a/b/c). 프로세스 하나는 사업장 하나를
# 담당하므로, 기동할 때 set_endpoint()로 자기 CSE에 바인딩한다.
# IN-AE는 중앙 CSE로 바인딩한다.
# ─────────────────────────────────────────────────────────

# 이 프로세스의 '기본' CSE. MN-AE는 자기 사업장 CSE, IN-AE는 중앙 CSE다.
_BASE_URL = config.BASE_URL
_CSE_NAME = config.CSE_NAME

# 중앙 CSE. MN-AE가 FL 제어·드롭박스에 접근할 때 쓴다.
_IN_BASE_URL = config.IN_BASE_URL
_IN_CSE_NAME = config.IN_CSE_NAME

# 경로가 이 접두사로 시작하면 중앙 CSE로 보낸다.
# 사업장 CSE와 중앙 CSE 주소가 다르므로 어느 쪽인지 표시가 필요하다.
IN_PREFIX = "@in/"

# 바인딩할 때마다 엔드포인트를 찍을지.
# 사업장을 오가며 수십 번 바인딩하므로 기본은 끔이다. 매번 찍으면
# 정작 봐야 할 ✓/✗ 가 묻힌다. 디버깅할 때만 FL_VERBOSE_BIND=1 로 켠다.
_VERBOSE_BIND = os.getenv("FL_VERBOSE_BIND", "0") == "1"


_ORIGINATOR = None


def set_originator(aeid: Optional[str]) -> None:
    """
    이 프로세스가 CSE에 보낼 X-M2M-Origin 을 정한다.

    지금까지는 모든 요청이 config.ORIGINATOR(기본 CAdmin)로 나갔다.
    관리자 신원은 ACP를 전부 통과하므로, 정책을 걸어 두어도 실제로는
    아무것도 막히지 않았다. 사업장 간 격리가 설정만 돼 있고 발동하지
    않는 상태였다.

    각 AE가 자기 AE-ID(CTRAINING-AE-1, CIN-AE …)로 요청해야 ACP가 의미를 갖는다.
    None 을 주면 기본값으로 돌아간다.
    """
    global _ORIGINATOR
    _ORIGINATOR = aeid
    if aeid:
        print(f"  [oneM2M] Originator -> {aeid}")


def originator() -> str:
    return _ORIGINATOR or config.ORIGINATOR


def set_endpoint(base_url: str, cse_name: str) -> None:
    """이 프로세스의 기본 CSE를 지정한다."""
    global _BASE_URL, _CSE_NAME
    _BASE_URL = base_url.rstrip("/")
    _CSE_NAME = cse_name
    if _VERBOSE_BIND:
        print(f"  [oneM2M] 기본 CSE -> {_BASE_URL}/{_CSE_NAME}")


def set_in_endpoint(base_url: str, cse_name: str) -> None:
    """중앙 CSE를 지정한다. @in/ 으로 시작하는 경로가 이쪽으로 간다."""
    global _IN_BASE_URL, _IN_CSE_NAME
    _IN_BASE_URL = base_url.rstrip("/")
    _IN_CSE_NAME = cse_name
    if _VERBOSE_BIND:
        print(f"  [oneM2M] 중앙 CSE -> {_IN_BASE_URL}/{_IN_CSE_NAME}")


def base_url() -> str:
    return _BASE_URL


def cse_name() -> str:
    return _CSE_NAME


def in_cse_name() -> str:
    return _IN_CSE_NAME


def in_path(relative: str) -> str:
    """중앙 CSE 안의 경로를 만든다. 예: in_path("IN-AE/cnt-fl-control")"""
    return f"{IN_PREFIX}{_IN_CSE_NAME}/{relative.lstrip('/')}"


def _url(path: str) -> str:
    """리소스 경로 -> 전체 URL. @in/ 접두사면 중앙 CSE로 보낸다."""
    p = str(path).lstrip("/")
    if p.startswith(IN_PREFIX):
        return f"{_IN_BASE_URL}/{p[len(IN_PREFIX):]}"
    return f"{_BASE_URL}/{p}"


def strip_prefix(path: str) -> str:
    """로그 출력용. @in/ 표시를 떼어낸다."""
    p = str(path).lstrip("/")
    return p[len(IN_PREFIX):] if p.startswith(IN_PREFIX) else p


_WARNED_INSECURE = False


def _verify():
    """
    TLS 검증 여부. 끄면 urllib3가 요청마다 경고를 찍어 로그를 덮으므로
    한 번만 알리고 이후 경고는 끈다. 끈 사실 자체는 반드시 보이게 둔다.
    """
    global _WARNED_INSECURE
    v = getattr(config, "TLS_VERIFY", True)
    if not v and not _WARNED_INSECURE:
        _WARNED_INSECURE = True
        print("  ⚠ TLS 검증을 껐다 (FL_TLS_VERIFY=0). 중간자 공격을 막지 못한다.")
        try:
            import urllib3
            urllib3.disable_warnings(
                urllib3.exceptions.InsecureRequestWarning)
        except Exception:
            pass
    return v


def _auth_for(url: str):
    """
    이 URL이 어느 CSE로 가는지 보고 HTTP 기본 인증 정보를 고른다.

    샌드박스의 MN-CSE는 열려 있지만 클라우드 IN-CSE는 401을 돌려준다.
    두 CSE의 자격증명이 다를 수 있으므로 URL로 구분한다.
    설정이 없으면 None이고, requests는 auth=None을 인증 없음으로 처리한다.
    """
    try:
        if _IN_BASE_URL and url.startswith(_IN_BASE_URL):
            return config.in_auth()
        return config.mn_auth()
    except Exception:
        return None


def _request_with_retry(method: str, url: str, **kwargs) -> requests.Response:
    delay = RETRY_DELAY
    last_exc = None
    kwargs.setdefault("verify", _verify())
    kwargs.setdefault("auth", _auth_for(url))
    for attempt in range(MAX_RETRY):
        try:
            return requests.request(method, url, **kwargs)
        except requests.exceptions.SSLError as e:
            # 샌드박스가 IP 기반 자체 서명 인증서를 쓰면 여기서 막힌다.
            print(f"  ✗ TLS 검증 실패: {url}")
            print("    자체 서명 인증서라면 FL_TLS_VERIFY=0 으로 실행하라.")
            raise e
        except (requests.exceptions.Timeout, requests.exceptions.ConnectionError) as e:
            last_exc = e
            if attempt < MAX_RETRY - 1:
                print(f"  ⏳ retry {attempt+1}/{MAX_RETRY-1} ({delay:.1f}s): {type(e).__name__}")
                time.sleep(delay)
                delay *= RETRY_BACKOFF
    raise last_exc


# -------------------------
# Headers
# -------------------------

_RI_COUNTER = itertools.count(1)


def _next_request_id() -> str:
    """
    X-M2M-RI (Request Identifier).

    TS-0009 HTTP 바인딩에서 모든 요청에 필수이며 요청마다 고유해야 한다.
    기존 구현은 공통 헤더에 아예 넣지 않았다. 관대한 CSE에서는 통과하지만
    표준 적합성을 보는 쪽에서는 가장 먼저 지적되는 항목이다.
    """
    return f"{os.getpid()}-{time.time_ns()}-{next(_RI_COUNTER)}"


def _headers(ty: Optional[int] = None, body: bool = False,
             origin: Optional[str] = None) -> Dict[str, str]:
    """
    요청 헤더를 만든다.

    ty를 주면 CREATE용 Content-Type(application/json;ty=N)을 붙인다.
    body=True면 ty 없는 본문 요청(UPDATE)이므로 Content-Type만 붙인다.
    둘 다 아니면 Content-Type을 넣지 않는다. RETRIEVE나 DELETE에
    Content-Type을 실으면 CSE가 요청을 거부한다.

    origin을 주면 이 요청에만 그 신원을 쓴다. 평소에는 프로세스 전역
    originator()로 충분하지만, 전역 신원으로는 아예 불가능한 요청이 있다.
    <pollingChannel> 생성이 그렇다. ACME는 부모 AE 본인만 허용하고
    관리자(CAdmin)조차 거부한다.

        acme/resources/AE.py:103-106
        if childResource.ty == ResourceTypes.PCH:
            # Check correct originator. Even the ADMIN is not allowed that
            if self.aei != originator:
                raise ORIGINATOR_HAS_NO_PRIVILEGE(...)
    """
    h = config.HEADERS.copy()
    h["X-M2M-RI"] = _next_request_id()
    h["X-M2M-Origin"] = origin or originator()
    if ty is not None:
        h["Content-Type"] = f"application/json;ty={ty}"
    elif body:
        h["Content-Type"] = "application/json"
    return h


# -------------------------
# ACP
# -------------------------

def create_acp(
    parent_path: str,
    acp_name: str,
    pv_rules: List[Dict],
    pvs_rules: List[Dict],
) -> Optional[str]:
    """
    ACP 생성 후 ri 반환.
    pv_rules/pvs_rules 예시:
      [{"acor": ["CAdmin", "CIN-AE"], "acop": 63}]
    """
    url = _url(parent_path)
    payload = {
        "m2m:acp": {
            "rn": acp_name,
            "pv":  {"acr": pv_rules},
            "pvs": {"acr": pvs_rules},
        }
    }
    try:
        r = _request_with_retry("post", url, json=payload, headers=_headers(1), timeout=10)
        if r.status_code == 201:
            ri = r.json().get("m2m:acp", {}).get("ri")
            return ri
        elif r.status_code == 409:
            # 이미 존재 → ri 조회
            get_url = _url(f"{parent_path}/{acp_name}")
            rg = _request_with_retry("get", get_url, headers=_headers(), timeout=10)
            if rg.status_code == 200:
                return rg.json().get("m2m:acp", {}).get("ri")
        print(f"✗ ACP create failed: {r.status_code} - {r.text}")
        return None
    except Exception as e:
        print(f"✗ ACP create error: {e}")
        return None


def update_acpi(resource_path: str, acpi: List[str]) -> bool:
    """
    기존 컨테이너에 acpi 연결 (PUT/UPDATE).
    acpi: ACP ri 목록 e.g. ["1-20260309T221953xxxx"]
    """
    url = _url(resource_path)
    payload = {"m2m:cnt": {"acpi": acpi}}
    try:
        r = _request_with_retry("put", url, json=payload,
                                headers=_headers(body=True), timeout=10)
        return r.status_code in (200, 204)
    except Exception as e:
        print(f"✗ update_acpi error: {e}")
        return False


# -------------------------
# AE / CNT / CIN
# -------------------------

def create_application(ae_name: str) -> Optional[Dict]:
    url = _url(_CSE_NAME)
    payload = {
        "m2m:ae": {
            "rn": ae_name,
            "api": f"N.{ae_name}.app",
            "rr": True,
            "srv": ["2a"],
        }
    }
    try:
        r = _request_with_retry("post", url, json=payload, headers=_headers(2), timeout=10)
        if r.status_code in (201, 409):
            return r.json() if r.status_code == 201 else {"exists": True}
        print(f"✗ AE create failed: {r.status_code} - {r.text}")
        return None
    except Exception as e:
        print(f"✗ AE create error: {e}")
        return None


def set_point_of_access(ae_path: str, poa: List[str],
                        reachable: bool = True) -> bool:
    """
    AE의 pointOfAccess(poa)와 requestReachability(rr)를 갱신한다.

    rr=true 는 "CSE가 나를 직접 호출해도 된다"는 선언인데, poa 가 비어 있으면
    CSE는 어디로 가야 할지 모른다. 그 상태로 구독을 만들면 CSE가 알림 대상을
    검증하지 못해 "Verification request failed" 로 거부한다.

    AE 자신이 자기 수신 주소를 알고 있으므로, 기동 시점에 스스로 등록한다.
    """
    payload = {"m2m:ae": {"poa": poa, "rr": reachable}}
    try:
        r = _request_with_retry("put", _url(ae_path), json=payload,
                                headers=_headers(body=True), timeout=10)
    except Exception as e:
        print(f"  ✗ poa 갱신 오류: {e}")
        return False

    if r.status_code in (200, 204):
        print(f"  ✓ {strip_prefix(ae_path)} poa={poa}")
        return True
    print(f"  ✗ poa 갱신 실패: {r.status_code} {r.text[:160]}")
    return False


def delete_resource(resource_path: str) -> bool:
    """리소스 삭제 (존재하지 않으면 True 반환)."""
    url = _url(resource_path)
    try:
        r = _request_with_retry("delete", url, headers=_headers(), timeout=10)
        return r.status_code in (200, 202, 204, 404)
    except Exception as e:
        print(f"✗ DELETE error: {e}")
        return False


def create_container(
    parent_path: str,
    container_name: str,
    mni: int = 1000,
    mbs: int = 10_000_000,
    acpi: Optional[List[str]] = None,
    labels: Optional[List[str]] = None,
) -> Optional[Dict]:
    """
    Container 생성.
    mbs: max byte size (기본 10MB — 가중치 CIN이 들어갈 여유)
    acpi: ACP ri 목록. 지정 시 생성과 동시에 acpi 연결 시도,
          실패하면 생성 후 UPDATE로 재시도.
    labels: 리소스 라벨(lbl). 경로를 박아 두는 대신 디스커버리로 찾게
          해 주는 열쇠다. 예: ["site:siteC", "type:local-model"].
    """
    url = _url(parent_path)
    cnt_obj: Dict[str, Any] = {"rn": container_name, "mni": mni, "mbs": mbs}
    if acpi:
        cnt_obj["acpi"] = acpi
    if labels:
        cnt_obj["lbl"] = list(labels)

    payload = {"m2m:cnt": cnt_obj}
    try:
        r = _request_with_retry("post", url, json=payload, headers=_headers(3), timeout=10)
        if r.status_code in (201, 409):
            result = r.json() if r.status_code == 201 else {"exists": True}

            # acpi가 있는데 생성 응답에 acpi가 없으면 UPDATE로 재시도
            if acpi:
                created_acpi = (r.json().get("m2m:cnt", {}).get("acpi", [])
                                if r.status_code == 201 else [])
                if not created_acpi:
                    resource_path = f"{parent_path}/{container_name}"
                    ok = update_acpi(resource_path, acpi)
                    if not ok:
                        print(f"  ⚠ acpi UPDATE 실패: {resource_path}")

            return result
        print(f"✗ CNT create failed: {r.status_code} - {r.text}")
        return None
    except Exception as e:
        print(f"✗ CNT create error: {e}")
        return None


def create_content_instance(
    container_path: str,
    content: Any,
    labels: Optional[List[str]] = None,
    resource_name: Optional[str] = None,
    origin: Optional[str] = None,
) -> Optional[Dict]:
    """
    resource_name을 주면 CIN의 rn을 직접 지정한다.
    가중치 조각처럼 수신측이 discovery 없이 이름으로 바로 RETRIEVE 해야 하는
    경우에 쓴다.
    """
    url = _url(container_path)
    con_str = (json.dumps(content, separators=(",", ":"), ensure_ascii=False)
               if isinstance(content, (dict, list)) else str(content))

    cin_obj: Dict[str, Any] = {"con": con_str}

    # contentInfo. con에 무엇이 들어 있는지 리소스만 보고 알 수 있게 한다.
    # dict/list를 JSON 문자열로 직렬화해 넣으므로 application/json:0 이다.
    cnf = getattr(config, "CIN_CONTENT_INFO", None)
    if cnf and isinstance(content, (dict, list)):
        cin_obj["cnf"] = cnf

    if resource_name:
        cin_obj["rn"] = resource_name
    if labels:
        cin_obj["lbl"] = labels

    payload: Dict[str, Any] = {"m2m:cin": cin_obj}

    try:
        r = _request_with_retry("post", url, json=payload,
                                headers=_headers(4, origin=origin), timeout=45)
        if r.status_code == 201:
            return r.json()
        print(f"✗ CIN create failed: {r.status_code} - {r.text}")
        return None
    except Exception as e:
        print(f"✗ CIN create error: {e}")
        return None


def get_latest_content_instance(container_path: str) -> Optional[Dict]:
    url = _url(f"{container_path}/la")
    try:
        r = _request_with_retry("get", url, headers=_headers(), timeout=10)
        if r.status_code == 200:
            return r.json()
        return None
    except Exception as e:
        print(f"✗ get_latest CIN error: {e}")
        return None


def probe_resource(resource_path: str):
    """
    RETRIEVE를 보내고 (status_code, 본문 일부)를 돌려준다.

    get_resource는 200이 아니면 조용히 None을 준다. 그래서 401(인증)과
    400(헤더 문제)과 404(경로 없음)를 구분할 수 없었다. 진단용으로 쓴다.
    """
    url = _url(resource_path)
    try:
        r = _request_with_retry("get", url, headers=_headers(), timeout=10)
        return r.status_code, r.text[:200]
    except Exception as e:
        return None, f"{type(e).__name__}: {e}"


def get_resource(resource_path: str) -> Optional[Dict]:
    url = _url(resource_path)
    try:
        r = _request_with_retry("get", url, headers=_headers(), timeout=10)
        if r.status_code == 200:
            return r.json()
        return None
    except Exception as e:
        print(f"✗ get_resource error: {e}")
        return None


# -------------------------
# Subscription
# -------------------------

def create_subscription(
    parent_path: str,
    subscription_name: str,
    notification_uri: str,
    event_types: List[int],
    use_nct: Optional[int] = None,
) -> Optional[Dict]:
    url = _url(parent_path)
    sub_obj: Dict[str, Any] = {
        "rn": subscription_name,
        "nu": [notification_uri],
        "enc": {"net": event_types},
    }
    if use_nct is not None:
        sub_obj["nct"] = use_nct

    payload = {"m2m:sub": sub_obj}
    try:
        r = _request_with_retry("post", url, json=payload, headers=_headers(23), timeout=10)
        if r.status_code in (201, 409):
            return r.json() if r.status_code == 201 else {"exists": True}
        print(f"✗ SUB create failed: {r.status_code} - {r.text}")
        return None
    except Exception as e:
        print(f"✗ SUB create error: {e}")
        return None


def delete_subscription(parent_path: str, subscription_name: str) -> bool:
    url = _url(f"{parent_path}/{subscription_name}")
    try:
        r = _request_with_retry("delete", url, headers=_headers(), timeout=10)
        return r.status_code in (200, 404)
    except Exception:
        return False


# -------------------------
# Discovery (label-based)
# -------------------------

def discover_uril_by_label(container_path: str, label: str, lim: int = 20) -> List[str]:
    base = _url(container_path)
    candidates = [
        f"{base}?fu=1&ty=4&lim={lim}&lbl={label}",
        f"{base}?fu=1&ty=4&lim={lim}&lbl={requests.utils.quote(label)}",
        f"{base}?fu=1&lbl={label}",
    ]
    for url in candidates:
        try:
            r = _request_with_retry("get", url, headers=_headers(), timeout=10)
            if r.status_code == 200:
                data = r.json()
                uril = data.get("m2m:uril", [])
                if isinstance(uril, list) and uril:
                    return uril
        except Exception:
            continue
    return []


def discover_by_labels(
    parent_path: str,
    labels: List[str],
    ty: Optional[int] = None,
    lim: int = 200,
    match_all: bool = True,
) -> List[str]:
    """
    라벨로 리소스를 찾는다 (?fu=1&lbl=...). 경로 문자열 목록을 돌려준다.

    왜 쓰나
      리소스 경로를 코드에 박아 두면, 로봇이 늘거나 이름 규칙이 바뀌면
      코드를 고쳐야 한다. 라벨로 찾으면 Isaac Sim 이 올린 그대로를
      그때그때 읽는다. 늦게 올라온 로봇도 다음 조회에서 잡힌다.

    labels    : ["site:siteC", "type:telemetry"] 처럼 여러 개
    ty        : 2 AE / 3 container / 4 contentInstance. None 이면 전부
    match_all : True 면 모든 라벨을 만족해야 한다(fo=1 AND).
                False 면 하나만 맞아도 된다(fo=2 OR).

    CSE 마다 fo 와 라벨 인코딩 처리가 조금씩 달라서 몇 가지 형태를
    차례로 시도하고, 처음 성공한 것을 쓴다.
    """
    if not labels:
        return []
    base = _url(parent_path)
    ty_q = f"&ty={ty}" if ty is not None else ""
    raw = "".join(f"&lbl={l}" for l in labels)
    enc = "".join(f"&lbl={requests.utils.quote(l, safe='')}" for l in labels)
    fo = "&fo=1" if match_all else "&fo=2"

    candidates = [
        f"{base}?fu=1&lim={lim}{ty_q}{fo}{raw}",
        f"{base}?fu=1&lim={lim}{ty_q}{fo}{enc}",
        f"{base}?fu=1&lim={lim}{ty_q}{raw}",
        f"{base}?fu=1&lim={lim}{ty_q}{enc}",
    ]
    # 다 실패했을 때 왜 실패했는지 남긴다. 403 이면 DISCOVERY 권한,
    # 200 인데 비었으면 라벨이 안 붙은 것이다. 둘은 고칠 곳이 다르다.
    LAST_DISCOVERY_STATUS.clear()
    for url in candidates:
        try:
            r = _request_with_retry("get", url, headers=_headers(), timeout=15)
            LAST_DISCOVERY_STATUS.append(r.status_code)
            if r.status_code != 200:
                continue
            uril = r.json().get("m2m:uril", [])
            if isinstance(uril, list) and uril:
                return uril
        except Exception as e:
            LAST_DISCOVERY_STATUS.append(type(e).__name__)
            continue
    return []


# discover_by_labels 가 마지막에 시도한 요청들의 응답 코드.
LAST_DISCOVERY_STATUS: List[Any] = []


def _relative_to_cse(raw: str, cse: str) -> List[str]:
    """디스커버리 결과 한 줄 -> cse 이름부터 시작하는 경로 조각들.

    CSE 마다 '/<cse-id>/<cse-rn>/...' 처럼 앞에 더 붙여 줄 수 있어서
    cse 이름 위치를 찾아 거기서부터 쓴다. 못 찾으면 그대로 둔다.
    """
    parts = [x for x in strip_prefix(raw).split("/") if x]
    if cse in parts:
        parts = parts[parts.index(cse):]
    return parts


def discover_telemetry(
    cse: str,
    site: str,
    cnt_name: str = "telemetry",
    type_label: str = "type:telemetry",
    combined_label: Optional[str] = None,
    lim: int = 200,
) -> Dict[str, str]:
    """
    한 사업장의 로봇 telemetry 컨테이너를 라벨로 찾는다. {로봇 AE 이름: 경로}

    Isaac Sim 이 다는 라벨 (합의한 형태):
      ["site:siteC", "robot:AMR_2", "ae:siteC_AMR_2",
       "type:telemetry", "siteC:telemetry"]

    [왜 'siteC:telemetry' 한 개로 먼저 찾나]
    lbl 을 여러 개 주면 CSE 구현에 따라 AND 가 아니라 OR 로 해석될 수
    있다(같은 조건 태그의 여러 값). OR 이면 site:siteC 만 붙은 것이 다
    걸린다. 우리 학습/분석 AE 의 cnt-local-model, cnt-net-status,
    cnt-verdict 에도 site:siteC 를 달았기 때문에 그것들이 '로봇' 으로
    잡혔다. 사업장과 종류를 한 라벨에 묶은 siteC:telemetry 는 해석이
    갈릴 여지가 없다. 그것으로 못 찾을 때만 두 라벨 조합을 쓴다.

    [왜 결과를 다시 확인하나]
    어느 쪽으로 찾았든 컨테이너를 한 번씩 읽어 lbl 을 직접 본다.
    site 와 type 이 둘 다 맞는 것만 남긴다. 로봇 이름은 'ae:' 라벨을
    우선 쓰고, 없으면 경로에서 부모 이름을 쓴다.
    """
    site_label = f"site:{site}"
    combined_label = combined_label or f"{site}:telemetry"

    uris: List[str] = []
    for labels in ([combined_label], [site_label, type_label]):
        got = discover_by_labels(cse, labels, ty=3, lim=lim)
        if got:
            uris = got
            break

    found: Dict[str, str] = {}
    for raw in uris:
        parts = _relative_to_cse(raw, cse)
        if len(parts) < 2:
            continue
        path = "/".join(parts) if parts[0] == cse else \
            f"{cse}/{parts[-2]}/{parts[-1]}"

        res = get_resource(path)
        cnt = (res or {}).get("m2m:cnt") or {}
        lbl = [str(x) for x in (cnt.get("lbl") or [])]
        ok = (combined_label in lbl) or (site_label in lbl and type_label in lbl)
        if not ok:
            continue

        robot = next((x[3:] for x in lbl if x.startswith("ae:")), parts[-2])
        # 한 로봇에 후보가 둘이면 이름이 telemetry 인 쪽을 쓴다.
        if robot in found and parts[-1] != cnt_name:
            continue
        found[robot] = path
    return found


def list_uril(container_path: str, lim: int = 50) -> List[str]:
    url = _url(f"{container_path}?fu=1&ty=4&lim={lim}")
    try:
        r = _request_with_retry("get", url, headers=_headers(), timeout=10)
        if r.status_code != 200:
            return []
        data = r.json()
        uril = data.get("m2m:uril", [])
        return uril if isinstance(uril, list) else []
    except Exception:
        return []


def discover(parent_path: str, ty: int, lim: int = 100) -> List[str]:
    """
    자손 리소스 디스커버리 (fu=1). ty로 종류를 고른다.
      ty=2 AE, ty=3 container, ty=4 contentInstance

    반환은 구조화된 리소스가 아니라 경로 문자열 목록(m2m:uril)이다.
    Isaac Sim이 만들어 둔 로봇 AE와 그 telemetry 컨테이너가
    실제로 있는지 확인하는 데 쓴다.
    """
    url = _url(f"{parent_path}?fu=1&ty={ty}&lim={lim}")
    try:
        r = _request_with_retry("get", url, headers=_headers(), timeout=15)
        if r.status_code != 200:
            return []
        uril = r.json().get("m2m:uril", [])
        return uril if isinstance(uril, list) else []
    except Exception as e:
        print(f"✗ discover error: {e}")
        return []


def discover_cins_after(container_path: str,
                        since_ct: Optional[str] = None,
                        lim: int = 300) -> List[str]:
    """
    컨테이너에서 since_ct 이후에 만들어진 contentInstance 경로만 가져온다.

    oneM2M TS-0001의 filterCriteria에 createdAfter(cra)가 있다. 표준이
    제공하는 필터이므로 전부 받아서 클라이언트가 거르는 것보다 낫다.
    받는 양도 줄고, "이번 라운드에 새로 들어온 것"이 서버 기준으로 정해진다.

    since_ct가 없으면 최근 lim개를 준다(첫 라운드).
    cra를 지원하지 않는 CSE면 빈 결과가 오므로, 호출하는 쪽에서
    전체를 받아 ct로 거르는 방식으로 되돌아간다.

    ct 형식은 oneM2M 절대시각이다. 예: 20261115T060913,588453
    """
    if not since_ct:
        return list_uril(container_path, lim=lim)

    # ct의 쉼표 뒤(마이크로초)는 cra가 받지 않는 구현이 있어 떼어낸다.
    stamp = str(since_ct).split(",")[0]

    # ACME 는 lim 을 cra 보다 '먼저', 그것도 오래된 것부터 적용한다.
    # 그래서 lim 이 컨테이너의 cni 보다 작으면 앞쪽(오래된) lim 개만
    # 후보가 되고, cra 로 거르면 하나도 안 남는다.
    #
    #   cni=71 일 때  lim=50  -> 0건
    #                 lim=100 -> 4건   (실측)
    #
    # lim 을 크게 고정하면 응답이 쓸데없이 커지므로, 컨테이너의 cni 를
    # 읽어 그만큼만 넉넉히 잡는다. GET 한 번이 늘지만 응답은 작다.
    cnt = get_resource(container_path)
    cni = ((cnt or {}).get("m2m:cnt") or {}).get("cni")
    try:
        need = int(cni) + 50
    except (TypeError, ValueError):
        need = 0
    lim = max(lim, need)

    url = _url(f"{container_path}?fu=1&ty=4&lim={lim}&cra={stamp}")
    try:
        r = _request_with_retry("get", url, headers=_headers(), timeout=15)
        if r.status_code != 200:
            return []
        uril = r.json().get("m2m:uril", [])
        return uril if isinstance(uril, list) else []
    except Exception as e:
        print(f"✗ discover_cins_after error: {e}")
        return []


def resource_exists(resource_path: str) -> bool:
    """단건 RETRIEVE로 존재 여부만 확인한다."""
    return get_resource(resource_path) is not None


def retrieve_cins_by_label(
    container_path: str,
    label: str,
    lim: int = 30,
    fallback_scan: int = 120,
    newest_first: bool = True,
) -> List[Dict]:
    def _ct(res: Dict) -> str:
        try:
            return (res.get("m2m:cin") or {}).get("ct", "") or ""
        except Exception:
            return ""

    matched: List[Dict] = []

    uril = discover_uril_by_label(container_path, label, lim=lim)
    if uril:
        for u in uril:
            cin = get_resource(u.lstrip("/"))
            if not cin or "m2m:cin" not in cin:
                continue
            if label in (cin["m2m:cin"].get("lbl") or []):
                matched.append(cin)
        if matched:
            matched.sort(key=_ct, reverse=newest_first)
            return matched

    uril2 = list_uril(container_path, lim=fallback_scan)
    if not uril2:
        return []

    for u in reversed(uril2):
        cin = get_resource(u.lstrip("/"))
        if not cin or "m2m:cin" not in cin:
            continue
        if label in (cin["m2m:cin"].get("lbl") or []):
            matched.append(cin)

    matched.sort(key=_ct, reverse=newest_first)
    return matched


def retrieve_latest_cin_by_label(
    container_path: str,
    label: str,
    lim: int = 30,
    fallback_scan: int = 80,
) -> Optional[Dict]:
    cins = retrieve_cins_by_label(
        container_path, label, lim=lim, fallback_scan=fallback_scan
    )
    return cins[0] if cins else None


def retrieve_first_cin_by_label(container_path: str, label: str) -> Optional[Dict]:
    return retrieve_latest_cin_by_label(container_path, label)