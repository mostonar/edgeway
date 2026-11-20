"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_app.py — ETSI GS MEC 011 (Edge Platform Application Enablement) client

Registers the site training app (TRAINING-AE-i) as a MEC app of that site's MEP, and
looks up the addresses of services the MEP provides (MEC013 location, MEC028 WLAN) in the service
registry.

[Order]
  1) Find the app instance ID   sandbox controller Applications API (not a MEC spec)
       GET  /{sbx}/sandbox-ctrl/v1/applications
     App instances are created by the sandbox scenario (edgeway). Here they are neither created
     nor deleted, only looked up by name (edgeway-fl-a) and MEP (mep-a).
     If missing, the scenario is an older version. Registration is skipped and FL continues.
  2) Ready notification          MEC011 Application Support
       POST /{sbx}/{mep}/mec_app_support/v2/applications/{id}/confirm_ready
            {"indication": "READY"}
  3) App registration            MEC011 Application Support
       POST /{sbx}/{mep}/mec_app_support/v2/registrations
            {"appName", "appInstanceId", "appProvider",
             "isInsByMec": false, "endpoint": {"uris": [...]}}
     The app runs outside the MEP, so isInsByMec=false, and then endpoint is required.
     This implementation returns 403 "Waiting for AppReadyConfirmation" if it is sent
     before 2). So it is sent after confirm_ready.
  4) Service discovery           MEC011 Service Management
       GET  /{sbx}/{mep}/mec_service_mgmt/v1/services
  5) Shutdown                    only the registration is deleted. The app instance belongs to the scenario and is left
       DELETE /{sbx}/{mep}/mec_app_support/v2/registrations/{id}

[Where it runs]
The training app runs not inside the MEP but on the site server (currently the lab server).
From MEC's point of view it is "an external MEC app registered with the MEP".

[Address rewriting]
URIs returned by the registry may be sandbox-internal names. Only the path is taken and
re-attached onto the host we connect to (FL_SANDBOX_HOST).
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


# ── 1) App instance ───────────────────────────────────

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
    """App instance ID created by the scenario. None if no instance with the same name and MEP exists."""
    for a in list_apps():
        if a.get("name") == name and a.get("nodeName") == mep and a.get("id"):
            return a["id"]
    print(f"  ✗ FL app {name}({mep}) not in scenario — check the sandbox scenario")
    return None


# ── 2) Ready notification ─────────────────────────────

def confirm_ready(mep: str, app_id: str) -> bool:
    """Try v2 first, v1 on 404. If already READY, treat it as success."""
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


# ── 3) App registration ───────────────────────────────

APP_PROVIDER = os.getenv("FL_MEC_APP_PROVIDER", "edgeway")

# MEC app name. One training app per MEP, so it is derived from the MEP name
# (mep-a -> edgeway-fl-a). It must match the scenario's app name for find_app to find it.
APP_PREFIX = os.getenv("FL_MEC_APP_PREFIX", "edgeway-fl")


def app_name(mep: str) -> str:
    return f"{APP_PREFIX}-{mep.split('-', 1)[-1]}"


def endpoint_for(port: int) -> str:
    """App address put into the MEC011 registration. Kept separate from FL_NOTIFY_HOST (oneM2M poa)."""
    host = os.getenv("FL_MEC_APP_HOST") or config.NOTIFY_HOST
    return f"http://{host}:{port}"


def register(mep: str, app_id: str, name: str, endpoint: str) -> bool:
    """
    Called after confirm_ready. Idempotent.
      No registration          POST
      Exists, same endpoint    leave it
      Exists, other endpoint   fix with PUT
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


# ── 4) Service discovery ──────────────────────────────

def services(mep: str) -> List[dict]:
    r = _req("GET", f"{mep_base(mep)}/mec_service_mgmt/v1/services")
    if r is None or r.status_code != 200:
        if r is not None:
            print(f"  ✗ service list: HTTP {r.status_code}")
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
    """Keep only the path of the registry URI and re-attach it to the host we connect to."""
    path = urlparse(uri).path.rstrip("/")
    return f"{_scheme()}://{config.SANDBOX_HOST}{path}"


# Service names differ per implementation (meep-loc-serv, mec013-1 ...), so if this word appears in the name,
# category or path, it is taken to be that service.
KEYS = {
    "location": ("loc", "013", "location"),
    "wai": ("wai", "028", "wlan"),
}


def discover(mep: str) -> Dict[str, dict]:
    """Only the ones found among {"location": {"name", "uri"}, "wai": {...}}."""
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
    ap = argparse.ArgumentParser(description="Check MEC011 registration and service discovery")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    ap.add_argument("--keep", action="store_true",
                    help="Do not delete the registration after the check (the app instance is kept either way)")
    ap.add_argument("--endpoint-host", default=None,
                    help="App address for MEC011 registration (default FL_MEC_APP_HOST, else FL_NOTIFY_HOST)")
    ap.add_argument("--base-port", type=int, default=5001,
                    help="Port for mn1. mn2, mn3 add 1 each")
    args = ap.parse_args()
    config.require_sandbox()
    print(f"sandbox-ctrl: {ctrl_base()}")
    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        name = app_name(mep)
        print(f"\n=== {node} / {mep} ===")
        app_id = find_app(name, mep)
        print(f"  app instance: {app_id}")
        if app_id:
            print(f"  confirm_ready: {confirm_ready(mep, app_id)}")
            idx = config.ALL_NODES.index(node) if node in config.ALL_NODES else 0
            ep = (f"http://{args.endpoint_host}:{args.base_port + idx}"
                  if args.endpoint_host else endpoint_for(args.base_port + idx))
            print(f"  registrations: {register(mep, app_id, name, ep)}  ({ep})")
        for s in services(mep):
            print(f"  service {s.get('serName')!s:20s} "
                  f"{(s.get('serCategory') or {}).get('name')!s:14s} "
                  f"{s.get('state')}  {_uri_of(s)}")
        print(f"  discovered: {discover(mep)}")
        if app_id and not args.keep:
            print(f"  registration deleted: {deregister(mep, app_id)}")


if __name__ == "__main__":
    main()
