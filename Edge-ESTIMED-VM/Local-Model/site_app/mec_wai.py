"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_wai.py — ETSI GS MEC 028 (WLAN Information API, WAI) 클라이언트

MEC 샌드박스의 각 MEP에 mec028-1(meep-wais)이 떠 있다. 로봇이 어느 AP에
붙어 있고 신호와 전송률이 어떤지는 여기서 받는다. 우리가 추측하지 않는다.

엔드포인트 (MEP 로컬 서비스)
  GET /{sbx}/{mep}/wai/v2/queries/ap/ap_information    AP 상태
  GET /{sbx}/{mep}/wai/v2/queries/sta/sta_information  단말 상태
  POST /{sbx}/{mep}/wai/v2/subscriptions               AssocSta / StaDataRate

쓰는 필드 (MEC028 v2.3.1)
  StaInfo.apAssociated.bssid       지금 붙어 있는 AP
  StaInfo.rssi.rssi                수신 신호 (dBm, 음수)
  StaInfo.staDataRate.staLastDataUplinkRate    상향 전송률 (kbps)
  ApInfo.bssLoad.channelUtilization            채널 점유 (0~255 = 0~100%)
  ApInfo.bssLoad.staCount                      붙어 있는 단말 수

네트워크 장애를 '만드는' 쪽은 이 파일이 아니다. 샌드박스 컨트롤러의
이벤트 API(net_scenarios.py)가 한다. 여기는 보는 쪽만 담당한다.
"""

import os
from typing import Dict, List, Optional

import requests

import config
import mec_topology as topo


# MEC028이 주는 channelUtilization은 255가 100%다 (IEEE 802.11 BSS Load).
CHU_FULL = 255.0

# 판정 문턱. 샌드박스 기본 Wi-Fi 모델 기준으로 잡았고 환경변수로 바꾼다.
RSSI_WARN = float(os.getenv("FL_RSSI_WARN_DBM", "-75"))   # 이하면 주의
RSSI_BAD = float(os.getenv("FL_RSSI_BAD_DBM", "-85"))     # 이하면 불량
CHU_WARN = float(os.getenv("FL_CHU_WARN_PCT", "70"))      # 채널 점유 %
UL_MIN_KBPS = float(os.getenv("FL_UL_MIN_KBPS", "500"))   # 상향 최소 전송률

TIMEOUT = float(os.getenv("FL_MEC_TIMEOUT_SEC", "8"))

# MEC011 서비스 발견으로 찾은 주소가 있으면 그걸 쓴다. mep -> base url
DISCOVERED: Dict[str, str] = {}


def _verify():
    return config.TLS_VERIFY


def wai_base(mep: str) -> str:
    """
    MEP 로컬 WAI 서비스의 베이스 URL.
    AdvantEDGE 인그레스가 /{sbx}/{mep}/wai 를 /wai 로 리라이트한다.
    """
    override = os.getenv(f"FL_WAI_BASE_{mep.replace('-', '_').upper()}")
    if override:
        return override.rstrip("/")
    if mep in DISCOVERED:
        return DISCOVERED[mep]
    scheme = os.getenv("FL_SANDBOX_SCHEME", "https")
    return (f"{scheme}://{config.SANDBOX_HOST}"
            f"/{config.SANDBOX_NAME}/{mep}/wai/v2")


def _get(mep: str, path: str) -> Optional[dict]:
    url = f"{wai_base(mep)}{path}"
    try:
        r = requests.get(url, timeout=TIMEOUT, verify=_verify(),
                         headers={"Accept": "application/json"})
    except Exception as e:
        print(f"  ✗ MEC028 {mep}{path}: {e}")
        return None
    if r.status_code != 200:
        print(f"  ✗ MEC028 {mep}{path}: HTTP {r.status_code}")
        return None
    try:
        return r.json()
    except Exception:
        print(f"  ✗ MEC028 {mep}{path}: JSON 파싱 실패")
        return None


# ── 조회 ────────────────────────────────────────────────

def ap_information(mep: str) -> List[dict]:
    """이 MEP가 보는 AP 목록. ApInfo 배열."""
    data = _get(mep, "/queries/ap/ap_information")
    if not data:
        return []
    # 구현에 따라 {"apInfo": [...]} 또는 바로 배열로 온다.
    if isinstance(data, list):
        return data
    for k in ("apInfo", "ApInfo", "apInfoList"):
        if isinstance(data.get(k), list):
            return data[k]
    return []


def sta_information(mep: str) -> List[dict]:
    """이 MEP가 보는 단말 목록. StaInfo 배열."""
    data = _get(mep, "/queries/sta/sta_information")
    if not data:
        return []
    if isinstance(data, list):
        return data
    for k in ("staInfo", "StaInfo", "staInfoList"):
        if isinstance(data.get(k), list):
            return data[k]
    return []


# ── 해석 ────────────────────────────────────────────────

def ap_load_by_mac(mep: str) -> Dict[str, dict]:
    """
    BSSID -> {"name", "channel_util_pct", "sta_count"}
    channelUtilization을 퍼센트로 바꿔 둔다. 255 척도를 그대로 쓰면
    문턱값을 읽을 수 없다.
    """
    out: Dict[str, dict] = {}
    for ap in ap_information(mep):
        bssid = ((ap.get("apId") or {}).get("bssid") or "").lower()
        if not bssid:
            continue
        load = ap.get("bssLoad") or {}
        chu = load.get("channelUtilization")
        out[bssid] = {
            "name": topo.poa_name_of_mac(bssid) or bssid,
            "channel_util_pct": (float(chu) / CHU_FULL * 100.0
                                 if chu is not None else None),
            "sta_count": load.get("staCount"),
            "channel": ap.get("channel"),
        }
    return out


def station(mep: str, ue_name: str,
            stas: Optional[List[dict]] = None) -> Optional[dict]:
    """
    UE 이름(robot-c2)으로 StaInfo 하나를 찾는다.
    MEC028은 MAC으로만 가리키므로 토폴로지 표로 변환한다.

    stas를 주면 그걸 쓴다. 로봇 여러 대를 볼 때 매번 조회하지 않기 위함이다.
    """
    mac = topo.UE_MAC.get(ue_name, "").lower()
    if not mac:
        return None
    for sta in (stas if stas is not None else sta_information(mep)):
        if ((sta.get("staId") or {}).get("macId") or "").lower() == mac:
            return sta
    return None


def link_state(mep: str, ue_name: str,
               stas: Optional[List[dict]] = None,
               loads: Optional[Dict[str, dict]] = None) -> dict:
    """
    로봇 하나의 링크 상태를 한 눈에 보이게 정리한다.

    반환
      verdict  "GOOD" | "DEGRADED" | "LOST"
      reasons  판정 근거 문장들 (데모 화면에 그대로 띄울 수 있게)
      ap       지금 붙어 있는 AP 이름
      rssi, ul_kbps, dl_kbps, channel_util_pct

    LOST는 AP 연결이 없거나 MEC028이 단말을 아예 못 보는 경우다.
    DEGRADED는 붙어 있지만 신호·전송률·채널 점유가 문턱을 넘은 경우다.
    """
    sta = station(mep, ue_name, stas=stas)
    if sta is None:
        return {"ue": ue_name, "verdict": "LOST", "ap": None,
                "rssi": None, "ul_kbps": None, "dl_kbps": None,
                "channel_util_pct": None,
                "reasons": ["MEC028에서 단말이 보이지 않는다"]}

    assoc = sta.get("apAssociated") or {}
    bssid = (assoc.get("bssid") or "").lower()
    ap_name = topo.poa_name_of_mac(bssid) or bssid or None

    rssi = (sta.get("rssi") or {}).get("rssi")
    rate = sta.get("staDataRate") or {}
    ul = rate.get("staLastDataUplinkRate")
    dl = rate.get("staLastDataDownlinkRate")

    chu = None
    if bssid:
        if loads is None:
            loads = ap_load_by_mac(mep)
        chu = (loads.get(bssid) or {}).get("channel_util_pct")

    reasons: List[str] = []
    if not bssid:
        reasons.append("붙어 있는 AP가 없다")
    if rssi is not None and float(rssi) <= RSSI_BAD:
        reasons.append(f"신호 {rssi}dBm (불량 기준 {RSSI_BAD:.0f})")
    elif rssi is not None and float(rssi) <= RSSI_WARN:
        reasons.append(f"신호 {rssi}dBm (주의 기준 {RSSI_WARN:.0f})")
    if ul is not None and float(ul) < UL_MIN_KBPS:
        reasons.append(f"상향 {ul}kbps (최소 {UL_MIN_KBPS:.0f})")
    if chu is not None and chu >= CHU_WARN:
        reasons.append(f"채널 점유 {chu:.0f}% (기준 {CHU_WARN:.0f}%)")

    # 붙어 있는 AP가 없으면 LOST, 문턱을 하나라도 넘었으면 DEGRADED.
    # 신호가 불량 기준 이하인 경우는 위에서 이미 reasons에 들어간다.
    if not bssid:
        verdict = "LOST"
    elif reasons:
        verdict = "DEGRADED"
    else:
        verdict = "GOOD"

    return {"ue": ue_name, "verdict": verdict, "ap": ap_name,
            "rssi": rssi, "ul_kbps": ul, "dl_kbps": dl,
            "channel_util_pct": chu, "reasons": reasons}


def site_link_states(node: str) -> List[dict]:
    """한 사업장 로봇 전체의 링크 상태."""
    mep = config.MEP_OF_NODE.get(node)
    if not mep:
        return []
    # 로봇마다 조회하면 대수만큼 왕복이 생긴다. 한 번 받아서 돌려 쓴다.
    stas = sta_information(mep)
    loads = ap_load_by_mac(mep)

    out = []
    for ae in config.robot_names(node):
        ue = topo.ae_to_ue(ae)
        if not ue:
            continue
        st = link_state(mep, ue, stas=stas, loads=loads)
        st["ae"] = ae
        out.append(st)
    return out


def best_target_ap(mep: str, ue_name: str,
                   exclude: Optional[List[str]] = None) -> Optional[str]:
    """
    핸드오버 목표 AP를 고른다.

    1순위는 단말이 직접 본 이웃 AP 보고(neighborReport)다. 단말이 실제로
    들을 수 있다는 증거이므로 추측보다 낫다.
    2순위는 같은 사업장 AP 중 채널 점유가 가장 낮은 것.
    3순위는 spare 사업장 AP (MEP가 바뀌므로 최후 수단).
    """
    skip = {s.lower() for s in (exclude or [])}
    skip |= {(topo.POA_MAC.get(s, "") or "").lower() for s in (exclude or [])}

    loads = ap_load_by_mac(mep)
    sta = station(mep, ue_name)

    # 1) 이웃 보고
    if sta:
        cur = ((sta.get("apAssociated") or {}).get("bssid") or "").lower()
        neigh = []
        for nb in sta.get("neighborReport") or []:
            bssid = (nb.get("bssid") or "").lower()
            if not bssid or bssid == cur or bssid in skip:
                continue
            name = topo.poa_name_of_mac(bssid)
            if not name or name in skip:
                continue
            chu = (loads.get(bssid) or {}).get("channel_util_pct")
            neigh.append((chu if chu is not None else 50.0, name))
        if neigh:
            neigh.sort()
            return neigh[0][1]

    # 2) 3) 토폴로지 후보에서 채널 점유가 낮은 쪽
    cands = topo.handover_candidates(ue_name, exclude=list(skip))
    scored = []
    for name in cands:
        mac = (topo.POA_MAC.get(name) or "").lower()
        chu = (loads.get(mac) or {}).get("channel_util_pct")
        # 이 MEP에서 안 보이는 AP(=spare)는 뒤로 밀되 후보로는 남긴다.
        scored.append((0 if mac in loads else 1,
                       chu if chu is not None else 50.0, name))
    if not scored:
        return None
    scored.sort()
    return scored[0][2]


# ── 단독 실행: 현재 링크 상태 덤프 ──────────────────────

def main() -> None:
    import argparse
    ap = argparse.ArgumentParser(
        description="MEC028으로 로봇 링크 상태를 본다")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES))
    args = ap.parse_args()

    config.require_sandbox()

    for node in [n.strip() for n in args.nodes.split(",") if n.strip()]:
        mep = config.MEP_OF_NODE.get(node, "?")
        site = config.SITE_OF_NODE.get(node, node)
        print(f"\n=== {node} / {site} / {mep} ===")
        print(f"  {wai_base(mep)}")

        aps = ap_load_by_mac(mep)
        if aps:
            print(f"  AP {len(aps)}개")
            for mac, info in aps.items():
                chu = info["channel_util_pct"]
                print(f"    {info['name']:22s} ch{info['channel']}  "
                      f"점유 {chu:.0f}%  단말 {info['sta_count']}대"
                      if chu is not None else
                      f"    {info['name']:22s} ch{info['channel']}")
        else:
            print("  ⚠ AP 정보를 못 받았다 (mec028-1이 떠 있는지 확인)")

        for st in site_link_states(node):
            mark = {"GOOD": "✓", "DEGRADED": "⚠", "LOST": "✗"}[st["verdict"]]
            print(f"    {mark} {st['ae']:14s} ({st['ue']:9s}) "
                  f"AP={st['ap']}  RSSI={st['rssi']}  UL={st['ul_kbps']}kbps")
            for r in st["reasons"]:
                print(f"        - {r}")


if __name__ == "__main__":
    main()
