# Diagnostic: MN-CSE routing check — relaying via the registrar (IN), and relaying through IN to another MN-CSE.
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
        print(f"    connection failed: {e}")
        return None, {}
    print(f"    HTTP {r.status_code}  RSC {r.headers.get('X-M2M-RSC')}  "
          f"Server {r.headers.get('Server')}")
    print("    " + (json.dumps(js, ensure_ascii=False) if js else r.text)[:300])
    return r, js


# 1) Address the site app actually uses (MEC033 → rule) and CSE-ID
ep, csi = {}, {}
for node in ("mn1", "mn2"):
    base, cse = mec_iot.resolve(node)
    ep[node] = (base, cse)
    _, js = show(f"{node} CSEBase", f"{base}/{cse}")
    csi[node] = (js.get("m2m:cb") or {}).get("csi")
    print(f"    csi = {csi[node]}")

if not csi["mn1"]:
    sys.exit("\nCould not read mn1 CSEBase. First check that the scenario is up and the address is correct.")

# 2) remoteCSE held by mn1 (IN-CSE registration)
base_a, cse_a = ep["mn1"]
_, js = show("mn1 remoteCSE list", f"{base_a}/{cse_a}?fu=1&ty=16")
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

# 3) Test 1: MN-a → IN (registrar)
if in_cb:
    show("Test1 MN-a → IN", f"{base_a}/~{in_cb}")
else:
    print("\nremoteCSE not visible, skipping Test1")

# 4) Test 2: MN-a → (via IN) MN-b
if csi["mn2"]:
    show("Test2 MN-a → MN-b", f"{base_a}/~{csi['mn2']}/{ep['mn2'][1]}")

if csi["mn1"] == csi["mn2"]:
    print("\nWarning: mep-a and mep-b MN-CSE have the same CSE-ID. Inter-CSE routing will not work.")
