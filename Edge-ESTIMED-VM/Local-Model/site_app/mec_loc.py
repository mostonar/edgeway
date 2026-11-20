"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_loc.py — ETSI GS MEC 013 (Location API) 클라이언트

MEC 샌드박스의 각 MEP에 mec013-1(위치 서비스)이 떠 있다. Isaac Sim 쪽
브리지가 로봇 pose 를 GIS engine 에 넣어 주므로(robot-a1 같은 UE 위치),
MEC013 은 로봇이 지금 어느 Zone, 어느 AP 에 있는지 알고 있다. 여기서는
그것을 읽기만 한다.

엔드포인트 (MEP 로컬 서비스, MEC013 v3)
  GET /{sbx}/{mep}/location/v3/queries/users?address=robot-a1   단말 위치
  GET /{sbx}/{mep}/location/v3/queries/zones                    Zone 목록

쓰는 필드
  UserInfo.address          단말 이름 (샌드박스 UE 이름)
  UserInfo.accessPointId    지금 붙어 있는 AP
  UserInfo.zoneId           지금 있는 Zone
  UserInfo.locationInfo     latitude[] / longitude[] (GIS 좌표)
  UserInfo.timestamp        위치 시각

판정(cnt-verdict)과 링크 기록(cnt-net-status)에 "어느 Zone 에서 났나"를
같이 남기려고 쓴다. 실패해도 FL 은 멈추지 않는다.
"""

import os
from typing import Dict, List, Optional
from urllib.parse import quote

import requests

import config

TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "4"))

# MEC011 서비스 발견으로 찾은 주소가 있으면 그걸 쓴다. mep -> base url
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
        print(f"  ✗ MEC013 {mep}{path}: JSON 파싱 실패")
        return None


def _users_of(data: Optional[dict]) -> List[dict]:
    """{"userList": {"user": [...]}} 또는 {"userInfo": {...}} 둘 다 받는다."""
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
    UE 하나의 위치. {"ue", "zone", "ap", "lat", "lon", "ts"} 또는 None.
    address 로 바로 묻고, 못 찾으면 전체 목록에서 이름으로 고른다.
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
    ap = argparse.ArgumentParser(description="MEC013 으로 로봇 위치를 본다")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    args = ap.parse_args()
    config.require_sandbox()
    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        print(f"\n=== {node} / {mep} ===  {loc_base(mep)}")
        for z in zones(mep):
            print(f"  zone {z.get('zoneId')}  AP {z.get('numberOfAccessPoints')}"
                  f"  단말 {z.get('numberOfUsers')}")
        for ae in config.robot_names(node):
            ue = topo.ae_to_ue(ae)
            loc = user_location(mep, ue) if ue else None
            print(f"  {ae:14s} ({ue}) -> {loc}")


if __name__ == "__main__":
    main()
