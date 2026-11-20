"""
Edge-ESTIMED-VM/Local-Model/site_app/mec_topology.py — MEC 샌드박스 시나리오(edgeway)의 토폴로지 표

MEC028은 BSSID(MAC)로 AP를 가리키고, 샌드박스 컨트롤러의 MOBILITY 이벤트는
요소 이름(site-c-poa-wifi-5)으로 가리킨다. 두 세계를 잇는 표가 필요하다.
값은 edgeway.yaml에서 뽑았다. 시나리오를 다시 만들면 MAC이 바뀌므로
아래 export_from_yaml()로 다시 뽑아 붙인다.

계층
  sandbox-operator
    site-a   (mep-a)  poa-wifi 1..7    로봇 robot-a1..a4 가 1..4에 붙어 있다
    site-b   (mep-b)  poa-wifi 1..7    로봇 robot-b1..b4
    site-c   (mep-c)  poa-wifi 1..10   로봇 robot-c1..c4
    spare    (mep-spare)  poa-wifi 1..13   로봇 없음 — 핸드오버 대피소

로봇 AE 이름(siteA_AMR_1)은 oneM2M 쪽 이름이고, UE 이름(robot-a1)은
샌드박스 쪽 이름이다. 같은 로봇이다.
"""

import os
from typing import Dict, List, Optional


# ── AP 이름 -> BSSID ────────────────────────────────────

POA_MAC: Dict[str, str] = {
    "site-a-poa-wifi-1": "026B5415002A",
    "site-a-poa-wifi-2": "02F9309CB169",
    "site-a-poa-wifi-3": "02B2CA9BFFEC",
    "site-a-poa-wifi-4": "022B1F81E380",
    "site-a-poa-wifi-5": "027E6015B78B",
    "site-a-poa-wifi-6": "02DB62A7490F",
    "site-a-poa-wifi-7": "0223BA1F66D0",
    "site-b-poa-wifi-1": "029617BD420D",
    "site-b-poa-wifi-2": "0213798C8AF7",
    "site-b-poa-wifi-3": "021718472624",
    "site-b-poa-wifi-4": "022A6290CCB3",
    "site-b-poa-wifi-5": "02154477CB00",
    "site-b-poa-wifi-6": "0278EEA8123F",
    "site-b-poa-wifi-7": "0285A3767026",
    "site-c-poa-wifi-1": "020B506C6675",
    "site-c-poa-wifi-2": "022B5EDEA2AA",
    "site-c-poa-wifi-3": "02D43412FA3C",
    "site-c-poa-wifi-4": "026632611B12",
    "site-c-poa-wifi-5": "028760CC32F7",
    "site-c-poa-wifi-6": "029D231DF388",
    "site-c-poa-wifi-7": "027D7920389F",
    "site-c-poa-wifi-8": "02BF2D97AA7B",
    "site-c-poa-wifi-9": "02FD982BC568",
    "site-c-poa-wifi-10": "02607DCC1C3F",
    "spare-poa-wifi-1": "02EF7EA1CE4D",
    "spare-poa-wifi-2": "02229F83A334",
    "spare-poa-wifi-3": "02AEA9974678",
    "spare-poa-wifi-4": "021053F30DD3",
    "spare-poa-wifi-5": "0297E2ADDB3F",
    "spare-poa-wifi-6": "02965B480A21",
    "spare-poa-wifi-7": "02BCAABBE068",
    "spare-poa-wifi-8": "0217699AD0DB",
    "spare-poa-wifi-9": "021FA4E8AC4A",
    "spare-poa-wifi-10": "0272CDE225DC",
    "spare-poa-wifi-11": "024699B76620",
    "spare-poa-wifi-12": "027AF27FABE8",
    "spare-poa-wifi-13": "023F415191B3",
}

# BSSID -> AP 이름. MEC028 응답을 이름으로 되돌릴 때 쓴다.
# MAC 대소문자가 섞여 오므로 소문자로 맞춰 둔다.
MAC_TO_POA: Dict[str, str] = {v.lower(): k for k, v in POA_MAC.items()}


# ── 로봇 UE ─────────────────────────────────────────────

UE_MAC: Dict[str, str] = {
    "robot-a1": "1f45fc2c89ab",
    "robot-a2": "10f7e3d9f6f7",
    "robot-a3": "7feaac85e068",
    "robot-a4": "fccf1b8997f9",
    "robot-b1": "8dc3ac1a70fc",
    "robot-b2": "0815e3ea2bbd",
    "robot-b3": "a2d2c01b928a",
    "robot-b4": "a7dfa55a65b6",
    "robot-c1": "e7bcdeaed0c7",
    "robot-c2": "fafec7a379fe",
    "robot-c3": "a07a77ce92a7",
    "robot-c4": "fea4f2dc21ff",
}

MAC_TO_UE: Dict[str, str] = {v.lower(): k for k, v in UE_MAC.items()}


# ── 사업장 ──────────────────────────────────────────────

SITE_OF_NODE = {"mn1": "site-a", "mn2": "site-b", "mn3": "site-c"}
MEP_OF_SITE = {"site-a": "mep-a", "site-b": "mep-b",
               "site-c": "mep-c", "spare": "mep-spare"}

# 핸드오버 대피소. 자기 사업장 AP가 전부 나쁠 때 여기로 보낸다.
SPARE_SITE = "spare"


def ae_to_ue(ae_name: str) -> Optional[str]:
    """
    oneM2M 로봇 AE 이름 -> 샌드박스 UE 이름.
      siteA_AMR_1 -> robot-a1
    AMR_UE_MAP 환경변수로 덮어쓸 수 있다("siteA_AMR_1=robot-a1,...").
    """
    override = _parse_map(os.getenv("AMR_UE_MAP", ""))
    if ae_name in override:
        return override[ae_name]

    try:
        site, _, idx = ae_name.split("_")          # siteA, AMR, 1
        letter = site.replace("site", "").lower()  # a
        ue = f"robot-{letter}{int(idx)}"
    except Exception:
        return None
    return ue if ue in UE_MAC else None


def ue_to_ae(ue_name: str) -> Optional[str]:
    """robot-a1 -> siteA_AMR_1"""
    try:
        tail = ue_name.split("-", 1)[1]            # a1
        letter, idx = tail[0], int(tail[1:])
        return f"site{letter.upper()}_AMR_{idx}"
    except Exception:
        return None


def poas_of_site(site: str) -> List[str]:
    """한 사업장의 AP 이름 목록. 번호 순."""
    pre = f"{site}-poa-wifi-"
    names = [k for k in POA_MAC if k.startswith(pre)]
    return sorted(names, key=lambda n: int(n[len(pre):]))


def site_of_poa(poa_name: str) -> Optional[str]:
    """site-c-poa-wifi-5 -> site-c"""
    if "-poa-wifi-" not in poa_name:
        return None
    return poa_name.split("-poa-wifi-")[0]


def poa_name_of_mac(mac: str) -> Optional[str]:
    return MAC_TO_POA.get((mac or "").lower())


def handover_candidates(ue_name: str, exclude: Optional[List[str]] = None) -> List[str]:
    """
    이 로봇이 옮겨갈 수 있는 AP 후보. 자기 사업장 AP를 먼저 보고,
    그다음 spare 사업장의 AP를 본다. exclude에 든 것은 뺀다.

    자기 사업장 안에서 옮기면 MN-CSE가 그대로이므로 FL 입장에서는
    경로만 바뀐다. spare로 넘어가면 MEP가 달라지므로 MEC021(AMS)이
    필요해진다 — 데모에서는 최후 수단이다.
    """
    ae = ue_to_ae(ue_name) or ""
    site = None
    for s in ("site-a", "site-b", "site-c"):
        if ae.lower().startswith(s.replace("-", "")):
            site = s
            break
    if site is None:
        letter = ue_name.split("-", 1)[-1][:1]
        site = f"site-{letter}"

    skip = set(exclude or [])
    own = [p for p in poas_of_site(site) if p not in skip]
    spare = [p for p in poas_of_site(SPARE_SITE) if p not in skip]
    return own + spare


def _parse_map(spec: str) -> Dict[str, str]:
    out: Dict[str, str] = {}
    for part in spec.split(","):
        part = part.strip()
        if "=" in part:
            k, v = part.split("=", 1)
            out[k.strip()] = v.strip()
    return out


# ── 표 재생성 ───────────────────────────────────────────

def export_from_yaml(path: str) -> str:
    """
    edgeway.yaml에서 POA_MAC / UE_MAC을 다시 뽑아 파이썬 코드로 돌려준다.
    시나리오를 새로 만들면 MAC이 전부 바뀌므로 그때 이걸 돌려서 위를 교체한다.

      python3 -c "import mec_topology as t; print(t.export_from_yaml('edgeway.yaml'))"
    """
    cur = None
    ents: Dict[str, str] = {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            s = line.strip()
            indent = len(line) - len(line.lstrip())
            if (s.startswith("- name: ") or s.startswith("name: ")) and indent <= 12:
                cur = s.split("name: ", 1)[1]
            if s.startswith("macId:") and cur:
                ents[cur] = s.split("macId:", 1)[1].strip().strip("'")

    def block(name: str, items: Dict[str, str]) -> str:
        body = "\n".join(f'    "{k}": "{v}",' for k, v in items.items())
        return f"{name} = {{\n{body}\n}}"

    aps = {k: v for k, v in ents.items() if "poa-wifi" in k}
    ues = {k: v for k, v in ents.items() if k.startswith("robot-")}
    return block("POA_MAC", aps) + "\n\n" + block("UE_MAC", ues)
