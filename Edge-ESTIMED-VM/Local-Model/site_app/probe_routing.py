# 진단: MN-CSE 라우팅 점검 — registrar(IN) 중계, IN 거쳐 다른 MN-CSE 로 가는 중계.
#   source .env.local && python3 Edge-ESTIMED-VM/Local-Model/site_app/probe_routing.py
import sys, json, uuid, requests, urllib3
import config, mec_iot
urllib3.disable_warnings()
V = config.TLS_VERIFY


def get(url):
    h = {"X-M2M-Origin": config.ORIGINATOR, "X-M2M-RI": uuid.uuid4().hex[:8],
         "X-M2M-RVI": "3", "Accept": "application/json"}
    r = requests.get(url, headers=h, verify=V, timeout=15)
    try:
        js = r.json()
    except ValueError:
        js = {}
    return r, js


def show(tag, url):
    print(f"\n[{tag}] {url}")
    try:
        r, js = get(url)
    except Exception as e:
        print(f"    연결 실패: {e}")
        return None, {}
    print(f"    HTTP {r.status_code}  RSC {r.headers.get('X-M2M-RSC')}  "
          f"Server {r.headers.get('Server')}")
    print("    " + (json.dumps(js, ensure_ascii=False) if js else r.text)[:300])
    return r, js


# 1) 사업장 앱이 실제로 쓰는 주소(MEC033 → 규칙)와 CSE-ID
ep, csi = {}, {}
for node in ("mn1", "mn2"):
    base, cse = mec_iot.resolve(node)
    ep[node] = (base, cse)
    _, js = show(f"{node} CSEBase", f"{base}/{cse}")
    csi[node] = (js.get("m2m:cb") or {}).get("csi")
    print(f"    csi = {csi[node]}")

if not csi["mn1"]:
    sys.exit("\nmn1 CSEBase 를 못 읽었다. 시나리오가 떠 있는지, 주소가 맞는지 먼저 확인.")

# 2) mn1 이 가진 remoteCSE (IN-CSE 등록)
base_a, cse_a = ep["mn1"]
_, js = show("mn1 remoteCSE 목록", f"{base_a}/{cse_a}?fu=1&ty=16")
uril = js.get("m2m:uril") or []
if isinstance(uril, str):
    uril = uril.split()
in_cb = None
for u in uril:
    _, j = get(f"{base_a}/{u}")
    csr = j.get("m2m:csr") or {}
    print(f"    {u}: csi={csr.get('csi')} cb={csr.get('cb')} cst={csr.get('cst')}")
    if csr.get("cst") == 1 or in_cb is None:
        in_cb = csr.get("cb") or f"{csr.get('csi')}/{csr.get('rn')}"

# 3) 테스트 1: MN-a → IN (registrar)
if in_cb:
    show("테스트1 MN-a → IN", f"{base_a}/~{in_cb}")
else:
    print("\nremoteCSE 가 안 보여서 테스트1 생략")

# 4) 테스트 2: MN-a → (IN 거쳐) MN-b
if csi["mn2"]:
    show("테스트2 MN-a → MN-b", f"{base_a}/~{csi['mn2']}/{ep['mn2'][1]}")

if csi["mn1"] == csi["mn2"]:
    print("\n주의: mep-a 와 mep-b MN-CSE 의 CSE-ID 가 같다. CSE 간 라우팅이 성립하지 않는다.")
