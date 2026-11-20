"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_iot.py — ETSI GS MEC 033 (IoT API) client

The site app asks MEC for its own MN-CSE address. The IoT platform registered with the MEP's
IoT service (meep-iot) is exactly that site's MN-CSE.

  GET /{sbx}/{mep}/iots/v1/registered_iot_platforms
      [{"iotPlatformId", "enabled",
        "userTransportInfo":          [{"name", "protocol", "endpoint": {...}}],
        "customServicesTransportInfo":[{"name", "protocol", "endpoint": {...}}]}]

  The transport name is the CSE name and the endpoint uris are the CSE address
  (meep-sss finds CSEs with the same fields).

[Order of use]
  1) If FL_MN_BASE_URL_<NODE> is given directly, use it. A value set by a person wins.
  2) Actually retrieve the CSEBase at the address from MEC033. If it responds, use it.
  3) Otherwise fall back to the existing rule (config.mn_endpoint). FL does not stop.

[Address rewriting]
Registered URIs may be sandbox-internal names. As with MEC011 service discovery, only the path
is taken and re-attached onto the host we connect to (FL_SANDBOX_HOST). That is why the check in 2)
is needed. If the path is internal-only, the retrieve fails and it goes to 3).

Disable: FL_MEC_IOT=0
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
        return None          # Internal addresses with only a host cannot be used from outside
    return f"{_scheme()}://{config.SANDBOX_HOST}{path}"


def candidates(mep: str) -> List[Tuple[str, str, str]]:
    """[(base_url, cse_name, iotPlatformId)] — HTTP CSEs registered with this MEP."""
    out = []
    for p in platforms(mep):
        if p.get("enabled") is False:
            continue
        trs = (p.get("customServicesTransportInfo") or []) + \
              (p.get("userTransportInfo") or [])
        for tr in trs:
            proto = str(tr.get("protocol") or "").upper()
            if proto and "HTTP" not in proto:
                continue      # MQTT etc. are not used by us
            uri, cse = _uri_of(tr), tr.get("name")
            base = _rebase(uri) if uri else None
            if base and cse:
                out.append((base, cse, str(p.get("iotPlatformId") or "")))
    # If there are several platforms, put the ones whose CSE name contains the MEP name first.
    out.sort(key=lambda c: mep not in c[1])
    return out


def _get_cse_base(base: str, cse: str, origin: Optional[str] = None):
    """RETRIEVE the CSEBase once. Without origin it goes out with the current identity."""
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
    """HTTP status of the CSEBase retrieve. None on connection failure / timeout."""
    r = _get_cse_base(base, cse)
    return None if r is None else r.status_code


def cse_answers(base: str, cse: str, origin: Optional[str] = None) -> bool:
    """Also check it really is a CSE (200 and the body is m2m:cb)."""
    r = _get_cse_base(base, cse, origin)
    return r is not None and r.status_code == 200 and "m2m:cb" in r.text


_cse_answers = cse_answers   # Old name


def resolve(node: str) -> Tuple[str, str]:
    """(base_url, cse_name) of a site node. MEC033 → verify → on failure, the existing rule."""
    fallback = config.mn_endpoint(node)
    mep = config.MEP_OF_NODE.get(node, "")
    if os.getenv(f"FL_MN_BASE_URL_{node.upper()}"):
        print(f"  [{node}] MN-CSE: using FL_MN_BASE_URL_{node.upper()} (MEC033 skipped)")
        return fallback
    if not _on() or not mep or not config.sandbox_ready():
        return fallback
    for base, cse, pid in candidates(mep):
        if _cse_answers(base, cse):
            same = (base, cse) == fallback
            print(f"  [{node}] MN-CSE: MEC033 {mep} platform {pid or '?'} → "
                  f"{base}/{cse}  {'(same as rule address)' if same else '(differs from rule address)'}")
            return base, cse
        print(f"  [{node}] MEC033 candidate {base}/{cse} no response")
    print(f"  [{node}] MN-CSE: not available from MEC033, using rule address "
          f"{fallback[0]}/{fallback[1]}")
    return fallback


def main() -> None:
    """Check: python3 Edge-ESTIMED-VM/Local-Model/site_app/mec_iot.py [mn1,mn2,mn3]"""
    import sys
    config.require_sandbox()
    nodes = (sys.argv[1] if len(sys.argv) > 1 else ",".join(config.ALL_NODES))
    for node in [n.strip() for n in nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        print(f"\n=== {node} / {mep} ===")
        for p in platforms(mep):
            print(f"  platform {p.get('iotPlatformId')} enabled={p.get('enabled')}")
        for c in candidates(mep):
            print(f"  candidate {c[0]}/{c[1]}")
        print(f"  result {resolve(node)}")


if __name__ == "__main__":
    main()
