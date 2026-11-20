"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_loc.py — ETSI GS MEC 013 (Location API) client

Each MEP in the MEC sandbox runs mec013-1 (location service). The bridge on the Isaac Sim
side feeds robot poses into the GIS engine (UE positions like robot-a1),
so MEC013 knows which zone and which AP each robot is in right now. Here we
only read that.

Endpoints (MEP-local service, MEC013 v3)
  GET /{sbx}/{mep}/location/v3/queries/users?address=robot-a1   terminal location
  GET /{sbx}/{mep}/location/v3/queries/zones                    zone list

Fields used
  UserInfo.address          terminal name (sandbox UE name)
  UserInfo.accessPointId    AP currently attached to
  UserInfo.zoneId           current zone
  UserInfo.locationInfo     latitude[] / longitude[] (GIS coordinates)
  UserInfo.timestamp        location timestamp

Used to also record "which zone it happened in" alongside verdicts (cnt-verdict) and link records
(cnt-net-status). FL does not stop if it fails.
"""

import os
from typing import Dict, List, Optional
from urllib.parse import quote

import requests

import config

TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "4"))

# If an address was found via MEC011 service discovery, use it. mep -> base url
DISCOVERED: Dict[str, str] = {}


def loc_base(mep: str) -> str:
    override = os.getenv(f"FL_LOC_BASE_{mep.replace('-', '_').upper()}")
    if override:
        return override.rstrip("/")
    if mep in DISCOVERED:
        return DISCOVERED[mep]
    scheme = os.getenv("FL_SANDBOX_SCHEME", "https")
    return (f"{scheme}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/{mep}/location/v3")


def _get(mep: str, path: str) -> Optional[dict]:
    url = f"{loc_base(mep)}{path}"
    try:
        r = requests.get(url, timeout=TIMEOUT, verify=config.TLS_VERIFY,
                         headers={"Accept": "application/json"})
    except Exception as e:
        print(f"  ✗ MEC013 {mep}{path}: {e}")
        return None
    if r.status_code != 200:
        print(f"  ✗ MEC013 {mep}{path}: HTTP {r.status_code}")
        return None
    try:
        return r.json()
    except Exception:
        print(f"  ✗ MEC013 {mep}{path}: JSON parse failed")
        return None


def _users_of(data: Optional[dict]) -> List[dict]:
    """Accepts both {"userList": {"user": [...]}} and {"userInfo": {...}}."""
    if not data:
        return []
    ul = data.get("userList") or {}
    users = ul.get("user") if isinstance(ul, dict) else None
    if isinstance(users, list):
        return users
    if isinstance(data.get("userInfo"), dict):
        return [data["userInfo"]]
    return []


def _first(v):
    if isinstance(v, list):
        return v[0] if v else None
    return v


def _summarize(u: dict) -> dict:
    li = u.get("locationInfo") or {}
    ts = u.get("timestamp") or {}
    return {
        "ue": u.get("address"),
        "zone": u.get("zoneId"),
        "ap": u.get("accessPointId"),
        "lat": _first(li.get("latitude")),
        "lon": _first(li.get("longitude")),
        "ts": ts.get("seconds") if isinstance(ts, dict) else ts,
    }


def user_location(mep: str, ue_name: str) -> Optional[dict]:
    """
    Location of one UE. {"ue", "zone", "ap", "lat", "lon", "ts"} or None.
    Ask directly by address; if not found, pick by name from the full list.
    """
    users = _users_of(_get(mep, f"/queries/users?address={quote(ue_name)}"))
    for u in users:
        if str(u.get("address", "")) == ue_name:
            return _summarize(u)
    for u in _users_of(_get(mep, "/queries/users")):
        if str(u.get("address", "")) == ue_name:
            return _summarize(u)
    return None


def zones(mep: str) -> List[dict]:
    data = _get(mep, "/queries/zones") or {}
    zl = data.get("zoneList") or {}
    z = zl.get("zone") if isinstance(zl, dict) else None
    return z if isinstance(z, list) else []


def main() -> None:
    import argparse
    import mec_topology as topo
    ap = argparse.ArgumentParser(description="Show robot location via MEC013")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    args = ap.parse_args()
    config.require_sandbox()
    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        print(f"\n=== {node} / {mep} ===  {loc_base(mep)}")
        for z in zones(mep):
            print(f"  zone {z.get('zoneId')}  AP {z.get('numberOfAccessPoints')}"
                  f"  users {z.get('numberOfUsers')}")
        for ae in config.robot_names(node):
            ue = topo.ae_to_ue(ae)
            loc = user_location(mep, ue) if ue else None
            print(f"  {ae:14s} ({ue}) -> {loc}")


if __name__ == "__main__":
    main()
