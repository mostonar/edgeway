"""
oneM2M Utility Functions (ACME CSE, Mca/Mcc)
- AE / CNT / CIN CRUD
- Create ACPs (Access Control Policy)
- Subscription
- Discovery with filterCriteria-like query (label-based)
- Retry: exponential back-off on timeout/connection errors
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
# Retry settings
# -------------------------
MAX_RETRY     = 4
RETRY_DELAY   = 3.0
RETRY_BACKOFF = 1.5

# ─────────────────────────────────────────────────────────
# Endpoints
#
# Each site has its own MN-CSE (mep-a/b/c). One process handles one site,
# so on startup it binds to its own CSE with set_endpoint().
# The IN-AE binds to the central CSE.
# ─────────────────────────────────────────────────────────

# This process's 'default' CSE. The MN-AE's is its site CSE; the IN-AE's is the central CSE.
_BASE_URL = config.BASE_URL
_CSE_NAME = config.CSE_NAME

# Central CSE. Used by the MN-AE to access FL control and the drop box.
_IN_BASE_URL = config.IN_BASE_URL
_IN_CSE_NAME = config.IN_CSE_NAME

# Paths starting with this prefix are sent to the central CSE.
# Site and central CSE addresses differ, so a marker for which one is needed.
IN_PREFIX = "@in/"

# Whether to print the endpoint on every bind.
# Binding happens dozens of times while moving between sites, so off by default. Printing every time
# buries the ✓/✗ you actually need to see. Turn on with FL_VERBOSE_BIND=1 only when debugging.
_VERBOSE_BIND = os.getenv("FL_VERBOSE_BIND", "0") == "1"


_ORIGINATOR = None


def set_originator(aeid: Optional[str]) -> None:
    """
    Decide the X-M2M-Origin this process sends to the CSE.

    Until now every request went out as config.ORIGINATOR (default CAdmin).
    The admin identity passes every ACP, so even with policies in place
    nothing was actually blocked. Site-to-site isolation was configured but never
    took effect.

    ACPs only mean something if each AE requests with its own AE-ID (CTRAINING-AE-1, CIN-AE …).
    Passing None reverts to the default.
    """
    global _ORIGINATOR
    _ORIGINATOR = aeid
    if aeid:
        print(f"  [oneM2M] Originator -> {aeid}")


def originator() -> str:
    return _ORIGINATOR or config.ORIGINATOR


def set_endpoint(base_url: str, cse_name: str) -> None:
    """Set this process's default CSE."""
    global _BASE_URL, _CSE_NAME
    _BASE_URL = base_url.rstrip("/")
    _CSE_NAME = cse_name
    if _VERBOSE_BIND:
        print(f"  [oneM2M] 기본 CSE -> {_BASE_URL}/{_CSE_NAME}")


def set_in_endpoint(base_url: str, cse_name: str) -> None:
    """Set the central CSE. Paths starting with @in/ go here."""
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
    """Build a path inside the central CSE. e.g. in_path("IN-AE/cnt-fl-control")"""
    return f"{IN_PREFIX}{_IN_CSE_NAME}/{relative.lstrip('/')}"


def _url(path: str) -> str:
    """Resource path -> full URL. With the @in/ prefix it goes to the central CSE."""
    p = str(path).lstrip("/")
    if p.startswith(IN_PREFIX):
        return f"{_IN_BASE_URL}/{p[len(IN_PREFIX):]}"
    return f"{_BASE_URL}/{p}"


def strip_prefix(path: str) -> str:
    """For log output. Strips the @in/ marker."""
    p = str(path).lstrip("/")
    return p[len(IN_PREFIX):] if p.startswith(IN_PREFIX) else p


_WARNED_INSECURE = False


def _verify():
    """
    Whether to verify TLS. When off, urllib3 prints a warning on every request and floods the log,
    so announce it once and silence later warnings. The fact that it is off is always kept visible.
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
    Pick HTTP basic auth credentials based on which CSE this URL goes to.

    The sandbox MN-CSEs are open but the cloud IN-CSE returns 401.
    The two CSEs may have different credentials, so they are told apart by URL.
    None if not configured; requests treats auth=None as no auth.
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
            # If the sandbox uses an IP-based self-signed certificate, it gets blocked here.
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

    Mandatory on every request in the TS-0009 HTTP binding and must be unique per request.
    The previous implementation did not include it in the common headers at all. Lenient CSEs let it pass,
    but it is the first thing flagged by anyone checking standard conformance.
    """
    return f"{os.getpid()}-{time.time_ns()}-{next(_RI_COUNTER)}"


def _headers(ty: Optional[int] = None, body: bool = False,
             origin: Optional[str] = None) -> Dict[str, str]:
    """
    Build request headers.

    If ty is given, add the CREATE Content-Type (application/json;ty=N).
    If body=True, it is a body request without ty (UPDATE), so only Content-Type is added.
    Otherwise Content-Type is not included. Putting Content-Type on RETRIEVE or DELETE
    makes the CSE reject the request.

    If origin is given, that identity is used for this request only. Normally the process-wide
    originator() is enough, but some requests are impossible with the global identity.
    <pollingChannel> creation is one. ACME allows only the parent AE itself and
    rejects even the admin (CAdmin).

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
    Create an ACP and return its ri.
    pv_rules/pvs_rules example:
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
            # Already exists → look up ri
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
    Attach acpi to an existing container (PUT/UPDATE).
    acpi: list of ACP ri, e.g. ["1-20260309T221953xxxx"]
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
    Update the AE's pointOfAccess (poa) and requestReachability (rr).

    rr=true declares "the CSE may call me directly", but if poa is empty
    the CSE does not know where to go. Creating a subscription in that state makes the CSE
    unable to verify the notification target, and it rejects with "Verification request failed".

    The AE knows its own receiving address, so it registers it itself at startup.
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
    """Delete a resource (returns True if it does not exist)."""
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
    Create a container.
    mbs: max byte size (default 10 MB — room for weight CINs)
    acpi: list of ACP ri. If given, try attaching acpi at creation time,
          and if that fails, retry with UPDATE after creation.
    labels: resource labels (lbl). The key that lets things be found via discovery
          instead of hard-coded paths. e.g. ["site:siteC", "type:local-model"].
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

            # If acpi was given but the creation response has no acpi, retry with UPDATE
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
    If resource_name is given, the CIN's rn is set explicitly.
    Used when the receiver must RETRIEVE directly by name without discovery,
    as with weight chunks.
    """
    url = _url(container_path)
    con_str = (json.dumps(content, separators=(",", ":"), ensure_ascii=False)
               if isinstance(content, (dict, list)) else str(content))

    cin_obj: Dict[str, Any] = {"con": con_str}

    # contentInfo. Lets you tell what is in con just by looking at the resource.
    # dict/list is serialized into a JSON string, so it is application/json:0.
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
    Send a RETRIEVE and return (status_code, part of the body).

    get_resource quietly returns None on anything other than 200, so 401 (auth),
    400 (header problem) and 404 (no path) could not be told apart. Used for diagnostics.
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
    Find resources by label (?fu=1&lbl=...). Returns a list of path strings.

    Why use it
      With resource paths hard-coded, the code must change whenever robots are added or
      the naming rule changes. Finding by label reads whatever Isaac Sim uploaded
      at that moment. Robots that come up late are picked up on the next lookup.

    labels    : several, like ["site:siteC", "type:telemetry"]
    ty        : 2 AE / 3 container / 4 contentInstance. None means all
    match_all : if True, all labels must match (fo=1 AND).
                If False, one match is enough (fo=2 OR).

    CSEs differ slightly in how they handle fo and label encoding, so several forms
    are tried in turn and the first that succeeds is used.
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
    # When everything fails, record why. 403 means DISCOVERY permission;
    # 200 but empty means the labels were not attached. The fixes are in different places.
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


# Response codes of the requests discover_by_labels tried last.
LAST_DISCOVERY_STATUS: List[Any] = []


def _relative_to_cse(raw: str, cse: str) -> List[str]:
    """One discovery result line -> path segments starting from the CSE name.

    Some CSEs prepend things like '/<cse-id>/<cse-rn>/...', so the position of the
    CSE name is found and used from there. If not found, it is left as is.
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
    Find a site's robot telemetry containers by label. {robot AE name: path}

    Labels attached by Isaac Sim (agreed format):
      ["site:siteC", "robot:AMR_2", "ae:siteC_AMR_2",
       "type:telemetry", "siteC:telemetry"]

    [Why search with the single 'siteC:telemetry' first]
    Given multiple lbl values, some CSE implementations interpret them as OR rather than AND
    (several values of the same condition tag). With OR, anything tagged only site:siteC
    matches. We had also put site:siteC on our training/analysis AEs' cnt-local-model, cnt-net-status
    and cnt-verdict, so those got picked up as 'robots'. siteC:telemetry, which binds site and type
    into one label, leaves no room for misreading.
    The two-label combination is used only when it finds nothing.

    [Why double-check the results]
    Whichever way they were found, each container is read once to check lbl directly.
    Only those where both site and type match are kept. The robot name comes from the 'ae:' label
    first, and if absent, from the parent name in the path.
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
        # If a robot has two candidates, the one named telemetry is used.
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
    Descendant resource discovery (fu=1). ty selects the type.
      ty=2 AE, ty=3 container, ty=4 contentInstance

    Returns a list of path strings (m2m:uril), not structured resources.
    Used to check that the robot AEs Isaac Sim created and their telemetry containers
    actually exist.
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
    Get only the contentInstance paths created after since_ct in a container.

    oneM2M TS-0001 filterCriteria includes createdAfter (cra). It is a filter the standard
    provides, so it beats fetching everything and filtering on the client.
    Less is received, and "what newly arrived this round" is decided by the server.

    Without since_ct, returns the latest lim (first round).
    A CSE that does not support cra returns an empty result, so the caller
    falls back to fetching everything and filtering by ct.

    ct format is oneM2M absolute time. e.g. 20261115T060913,588453
    """
    if not since_ct:
        return list_uril(container_path, lim=lim)

    # Some implementations do not accept the part after the comma (microseconds) in cra, so it is stripped.
    stamp = str(since_ct).split(",")[0]

    # ACME applies lim 'before' cra, and from the oldest.
    # So if lim is smaller than the container's cni, only the first (oldest) lim entries
    # are candidates, and filtering by cra leaves nothing.
    #
    #   With cni=71:  lim=50  -> 0 results
    #                 lim=100 -> 4 results   (measured)
    #
    # Fixing lim large makes responses needlessly big, so the container's cni is read
    # and lim is set just above it. One extra GET, but the response stays small.
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
    """Check existence only, with a single RETRIEVE."""
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