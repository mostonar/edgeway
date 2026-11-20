"""
Cloud/FL-FedAvg/setup_resources_standard.py — create the oneM2M FL resource structure (ACME, one CSE per MEP)

The MEC sandbox has 4 CSEs. Each site has one MN-CSE inside its MEP, and
there is one IN-CSE in the cloud. Each resource has to go onto a different CSE,
so this script switches the binding right before each creation.

  IN-CSE (cloud)                           config.in_endpoint()
    TRAINING-AE-1/2/3                      ← created by us. The site training apps are
                                             registered on both MN-CSE and IN-CSE.
                                             The IN side is the identity used to hold
                                             a <pollingChannel> and write to the drop box.
    IN-AE                                  ← created by us
      cnt-fl-control      round instructions (IN→MN)
      cnt-global-model    global weights (IN→MN)
      cnt-local-updates   local update collection (MN→IN)
        cnt-mn1/2/3       per-node dropbox
      cnt-fl-inference    global model validation set evaluation (written by IN-AE, read by the dashboard)

  MN-CSE (mep-a/b/c)                       config.mn_endpoint(node)
    siteA_AMR_1 … siteA_AMR_4              ← created by Isaac Sim. Only checked
      telemetry                            ← filled by Isaac. We read it
      status
    TRAINING-AE-1                          ← created by us. Training app
      cnt-local-model     round cache
      cnt-net-status      MEC028 snapshot / buffer state
      cnt-window-scores   per-round window anomaly scores (read by the dashboard; never leaves the site)
    ANALYSIS-AE-1                          ← created by us. Analysis app
      cnt-verdict         verdicts. Never leave the site

It matters that there are two kinds of AE.
  Robot AE   : registered on the CSE by Isaac Sim. Not created here. Only a warning if missing.
  FL app AE  : TRAINING-AE-1/2/3, ANALYSIS-AE-1/2/3, IN-AE.
              Nobody else creates them, so they are created here.

Reset (--clean) uses oneM2M DELETE only. Only resources we created are deleted.

ACP policies
  acp-fl-control    IN-AE all / TRAINING-AE-* RETRIEVE          (IN-CSE)
  acp-global-model  IN-AE all / TRAINING-AE-* RETRIEVE          (IN-CSE)
  acp-local-updates IN-AE all / TRAINING-AE-* CREATE            (IN-CSE)
  acp-dropbox-mnX   IN-AE all / only TRAINING-AE-X CREATE       (IN-CSE)
  acp-cache-mnX     TRAINING-AE-X all / IN-AE RETRIEVE          (MN-CSE of X)
  acp-verdict-mnX   analysis app all / training app·robot AE RETRIEVE (MN-CSE of X)
  No ACP is put on robot telemetry. Isaac owns it.

ACOP bitmask (TS-0001)
  CREATE 1 / RETRIEVE 2 / UPDATE 4 / DELETE 8 / NOTIFY 16 / DISCOVERY 32 / ALL 63

Usage
  python3 Cloud/FL-FedAvg/setup_resources_standard.py              create everything
  python3 Cloud/FL-FedAvg/setup_resources_standard.py --verify      check only
  python3 Cloud/FL-FedAvg/setup_resources_standard.py --clean       delete our resources and recreate
  python3 Cloud/FL-FedAvg/setup_resources_standard.py --nodes mn1,mn2
  python3 Cloud/FL-FedAvg/setup_resources_standard.py --scope in   IN-CSE side only (Cloud/FL-FedAvg)
  python3 Cloud/FL-FedAvg/setup_resources_standard.py --scope mn   MN-CSE side only (Edge-ESTIMED-VM/Local-Model)
"""

import argparse
import os
import sys
import time
from typing import Dict, List, Optional

import config
import onem2m_utils as om2m


ORIGIN_IN_AE = config.IN_AE_ID

ACOP_CREATE = 1
ACOP_RETRIEVE = 2
ACOP_UPDATE = 4
ACOP_DELETE = 8
ACOP_NOTIFY = 16
ACOP_DISCOVERY = 32
ACOP_ALL = 63

SETTLE = 0.3  # Time for the CSE to commit the creation


def origin_mn(node: str) -> str:
    """Originator of the training app. mn2 -> CTRAINING-AE-2"""
    return config.training_ae_id(node)


def all_mn_origins(nodes: List[str]) -> List[str]:
    return [origin_mn(n) for n in nodes]


# ════════════════════════════════════════════════════════
# CSE binding
# ════════════════════════════════════════════════════════

def bind_in() -> str:
    """Send subsequent requests to the IN-CSE (cloud). Returns the CSE name."""
    base, cse = config.in_endpoint()
    om2m.set_endpoint(base, cse)
    om2m.set_in_endpoint(base, cse)
    return cse


def bind_mn(node: str) -> str:
    """Send subsequent requests to that node's MEP MN-CSE. Returns the CSE name."""
    base, cse = config.mn_endpoint(node)
    om2m.set_endpoint(base, cse)
    in_base, in_cse = config.in_endpoint()
    om2m.set_in_endpoint(in_base, in_cse)
    return cse


def reachable(label: str) -> bool:
    """RETRIEVE the base of the currently bound CSE to see if it is alive."""
    cse = om2m.cse_name()
    code, body = om2m.probe_resource(cse)
    if code != 200:
        print(f"  ✗ {label}: {om2m.base_url()}/{cse}")
        if code is None:
            print(f"      connection failed — {body}")
        else:
            print(f"      HTTP {code}  {body}")
            hint = {
                400: "headers may be wrong (Content-Type ty on RETRIEVE)",
                401: "auth required — set FL_IN_USER / FL_IN_PASS",
                403: "forbidden — check X-M2M-Origin and the ACP",
                404: "no resource at path — check the CSE name",
            }.get(code)
            if hint:
                print(f"      → {hint}")
        return False
    print(f"  ✓ {label}: {om2m.base_url()}/{cse}")
    return True


# ════════════════════════════════════════════════════════
# AE creation (ours: IN-AE, TRAINING-AE-i, ANALYSIS-AE-i)
# ════════════════════════════════════════════════════════

def create_fl_ae(ae_name: str, origin: str) -> bool:
    """
    Register an FL application AE.

    rr=True means request reachable. Needed for the training app to receive the NOTIFYs
    the IN-AE sends. srv is the supported release versions (TS-0004).
    If it already exists (409), leave it.
    """
    cse = om2m.cse_name()
    payload = {
        "m2m:ae": {
            "rn": ae_name,
            "api": f"N{ae_name}",
            "rr": True,
            "srv": ["2a", "3"],
        }
    }
    headers = om2m._headers(2)
    headers["X-M2M-Origin"] = origin

    try:
        # Going through _request_with_retry applies TLS settings and basic auth together.
        r = om2m._request_with_retry(
            "post",
            om2m._url(cse),
            json=payload,
            headers=headers,
            timeout=10,
        )
    except Exception as e:
        print(f"  ✗ AE {ae_name}: {e}")
        return False

    if r.status_code == 201:
        print(f"  ✓ AE created: {ae_name}  (Origin {origin})")
        return True
    if r.status_code == 409:
        print(f"  = AE already exists: {ae_name}")
        return True
    print(f"  ✗ AE {ae_name}: {r.status_code} {r.text[:160]}")
    return False


# ════════════════════════════════════════════════════════
# Robot AE check (owned by Isaac Sim — not created here)
# ════════════════════════════════════════════════════════

def verify_robot_aes(node: str) -> Dict[str, bool]:
    """
    Find this site's robot AEs and telemetry containers on the currently bound MN-CSE.

    Isaac Sim registers them, so here we only list them via discovery (fu=1&ty=2)
    and report against the expected count. If missing, say to run Isaac first.
    """
    cse = om2m.cse_name()
    site = config.SITE_OF_NODE.get(node, node)
    want = config.ROBOTS_PER_NODE.get(node, 0)

    # First find telemetry containers by label. Do not rely on naming rules.
    # Search with 'siteC:telemetry' first, and re-check lbl on each result.
    # Our own containers also carry site: labels, so a query with multiple lbl values
    # would pick them up as robots too if the CSE interprets it as OR.
    labels = [config.site_telemetry_label(node)] + config.telemetry_labels(node)
    tel_paths = om2m.discover_telemetry(
        cse, site,
        cnt_name=config.ROBOT_TELEMETRY_CNT,
        type_label=config.LABEL_TYPE_TELEMETRY,
        combined_label=config.site_telemetry_label(node),
    )

    if tel_paths:
        print(f"  label discovery {labels}: {len(tel_paths)}/{want} robots")
        found: Dict[str, bool] = {}
        for rb in sorted(tel_paths):
            found[rb] = True
            print(f"    ✓ {tel_paths[rb]}")
        if len(found) < want:
            print(f"    ⚠ only {len(found)} of {want} robots are labeled.")
        return found

    # If there are no labels, fall back to finding by name (when Isaac does not attach labels yet).
    codes = ", ".join(str(c) for c in om2m.LAST_DISCOVERY_STATUS)
    print(f"  ⓘ nothing found by label {labels} (response {codes or 'none'}). "
          f"falling back to the naming rule")
    uril = om2m.discover(cse, ty=2, lim=200)
    robots = sorted({
        p.rstrip("/").split("/")[-1]
        for p in uril
        if p.rstrip("/").split("/")[-1].startswith(f"{site}_")
    })

    print(f"  robot AEs ({site}_*): {len(robots)}/{want} found")

    found: Dict[str, bool] = {}
    for rb in robots:
        tel = f"{cse}/{rb}/{config.ROBOT_TELEMETRY_CNT}"
        ok = om2m.resource_exists(tel)
        found[rb] = ok
        print(f"    {'✓' if ok else '✗'} {rb}/{config.ROBOT_TELEMETRY_CNT}")

    if not robots:
        print(f"    ⚠ no {site} robot AEs. Start Isaac Sim first.")
    elif len(robots) < want:
        print(f"    ⚠ only {len(robots)} of {want} robots registered.")

    return found


# ════════════════════════════════════════════════════════
# IN-CSE side ACPs
# ════════════════════════════════════════════════════════

def setup_in_acps(nodes: List[str]) -> Dict[str, str]:
    """
    Create ACPs for FL control on the IN-CSE. Returns {logical name: ACP ri}.

    pv is the access rule for resources this ACP is attached to; pvs is the rule
    for the ACP itself. If pvs is not narrow, anyone can change the policy.
    """
    print("\n=== IN-CSE ACP ===")
    cse = bind_in()
    admin = config.ORIGINATOR
    mn_origins = all_mn_origins(nodes)
    acps: Dict[str, str] = {}

    def mk(name: str, pv: List[Dict]) -> None:
        ri = om2m.create_acp(
            cse, name, pv,
            [{"acor": [admin, ORIGIN_IN_AE], "acop": ACOP_ALL}],
        )
        if ri:
            acps[name.replace("acp-", "")] = ri
            print(f"  ✓ {name}")
        else:
            print(f"  ✗ {name}")
        time.sleep(SETTLE)

    read_for_mn = [
        {"acor": [admin, ORIGIN_IN_AE], "acop": ACOP_ALL},
        {"acor": mn_origins,
         "acop": ACOP_RETRIEVE | ACOP_NOTIFY | ACOP_DISCOVERY},
    ]
    # The training app must subscribe to cnt-fl-control. ACME requires CREATE on the parent
    # to create a subscription. Plain CREATE would also allow creating round command CINs,
    # so it is narrowed to when the child type is a subscription (ty=23).
    mk("acp-fl-control", read_for_mn + [
        {"acor": mn_origins, "acop": ACOP_CREATE,
         "acod": [{"chty": [23]}]},
    ])
    mk("acp-global-model", read_for_mn)

    mk("acp-local-updates", [
        {"acor": [admin, ORIGIN_IN_AE], "acop": ACOP_ALL},
        {"acor": mn_origins, "acop": ACOP_CREATE | ACOP_DISCOVERY},
    ])

    # Per-round validation set evaluation. Written by IN-AE and read by the dashboard (the dashboard uses the
    # admin identity). Without an ACP, the IN-AE cannot create CINs when running with its own AE-ID.
    mk("acp-fl-inference", [
        {"acor": [admin, ORIGIN_IN_AE], "acop": ACOP_ALL},
    ])

    for node in nodes:
        mk(f"acp-dropbox-{node}", [
            {"acor": [admin, ORIGIN_IN_AE], "acop": ACOP_ALL},
            {"acor": [origin_mn(node)],
             "acop": ACOP_CREATE | ACOP_RETRIEVE},
        ])

    return acps


def setup_mn_acp(node: str) -> Optional[str]:
    """Create the local-cache ACP on the node's own MN-CSE."""
    cse = om2m.cse_name()
    ri = om2m.create_acp(
        cse, f"acp-cache-{node}",
        [
            {"acor": [config.ORIGINATOR, origin_mn(node)], "acop": ACOP_ALL},
            {"acor": [ORIGIN_IN_AE],
             "acop": ACOP_RETRIEVE | ACOP_DISCOVERY},
        ],
        [{"acor": [config.ORIGINATOR, origin_mn(node)], "acop": ACOP_ALL}],
    )
    print(f"  {'✓' if ri else '✗'} acp-cache-{node}")
    time.sleep(SETTLE)
    return ri


# ════════════════════════════════════════════════════════
# IN-CSE resources
# ════════════════════════════════════════════════════════

def setup_in_resources(nodes: List[str], acps: Dict[str, str]) -> None:
    """Create the IN-AE and the control / model / collection containers under it."""
    print("\n=== IN-CSE resources ===")
    cse = bind_in()

    create_fl_ae(config.IN_AE_NAME, ORIGIN_IN_AE)
    time.sleep(SETTLE)

    # Site training apps must also be registered as AEs on the IN-CSE.
    #
    # A <pollingChannel> is a child of an AE and only that AE can create it (not even the
    # admin). The campus firewall drops inbound traffic so the IN-CSE cannot call us directly,
    # and all round commands therefore come through that channel. Without the AE
    # here, the MN never receives commands. The drop-box ACP acor also points to this
    # AE-ID.
    #
    # This code did not exist until now. The AEs on the IN-CSE were made by hand
    # long ago, and if that CSE had been reset everything would have silently stopped.
    for node in nodes:
        create_fl_ae(config.training_ae_name(node),
                     config.training_ae_id(node))
        time.sleep(SETTLE)

    ae = f"{cse}/{config.IN_AE_NAME}"

    # Attach labels. They are the key that lets things be found via ?fu=1&lbl=... instead of
    # hard-coded paths. type: says what it is; scope:in means it is a central resource.
    def cnt(parent: str, name: str, mni: int, acp_key: Optional[str],
            kind: Optional[str] = None) -> None:
        om2m.create_container(
            parent, name, mni=mni,
            acpi=[acps[acp_key]] if acp_key and acps.get(acp_key) else None,
            labels=["scope:in", f"type:{kind or name.removeprefix('cnt-')}"],
        )
        print(f"  ✓ {name}  (mni={mni})")
        time.sleep(SETTLE)

    cnt(ae, "cnt-fl-control", 50, "fl-control")
    cnt(ae, "cnt-global-model", 50, "global-model")
    cnt(ae, "cnt-local-updates", 50, "local-updates")

    # Per-node dropbox. Separated so one node cannot see another node's updates.
    updates = f"{ae}/cnt-local-updates"
    for node in nodes:
        om2m.create_container(
            updates, f"cnt-{node}", mni=50,
            acpi=([acps[f"dropbox-{node}"]]
                  if acps.get(f"dropbox-{node}") else None),
            labels=["scope:in", "type:local-updates",
                    f"node:{node}", config.site_label(node)],
        )
        print(f"    ✓ cnt-local-updates/cnt-{node}")
        time.sleep(SETTLE)

    # Per-round global model validation set evaluation. Written by IN-AE and read by the dashboard.
    # Site verdicts do not come here (they are in ANALYSIS-AE-i/cnt-verdict on the MN-CSE).
    cnt(ae, "cnt-fl-inference", 200, "fl-inference")


# ════════════════════════════════════════════════════════
# MN-CSE resources
# ════════════════════════════════════════════════════════

def robot_ae_ids(cse: str, node: str) -> List[str]:
    """
    Read the AE-IDs of this site's robot AEs.

    Ask the CSE instead of guessing. Robot AEs are created by Isaac Sim and AE-IDs are
    assigned by the CSE, so inferring them from names is wrong. They go into the ACP acor,
    so even one character off means Isaac Sim cannot subscribe.
    """
    ids: List[str] = []
    for robot in config.robot_names(node):
        res = om2m.get_resource(f"{cse}/{robot}")
        aei = ((res or {}).get("m2m:ae") or {}).get("aei")
        if aei:
            ids.append(aei)
    return ids


def setup_analysis_ae(node: str, cse: str) -> None:
    """
    Create the analysis app AE and the verdict container.

    Structure
      <MN-CSE>/ANALYSIS-AE-i/cnt-verdict/<cin>

    Permissions
      Analysis app   full (its own resources)
      Judge app      RETRIEVE + NOTIFY + DISCOVERY (+ CREATE)  -> receives via subscription
      Robot AE       RETRIEVE + NOTIFY + DISCOVERY             -> receives via subscription
      Training app   RETRIEVE + DISCOVERY                      -> read by the dashboard

    The key point is that robots get no CREATE. Only the analysis app writes verdicts.
    Only the judge app (CJUDGE-AE-i) also gets CREATE, because some CSEs require
    CREATE to create a <subscription>. See config.VERDICT_SUB_CREATE.
    """
    ae_name = config.analysis_ae_name(node)
    ae_id = config.analysis_ae_id(node)

    if not create_fl_ae(ae_name, ae_id):
        print(f"  ✗ failed to create {ae_name} — skipping verdict containers")
        return
    time.sleep(SETTLE)

    robots = robot_ae_ids(cse, node)
    if robots:
        print(f"  robot AE-IDs: {', '.join(robots)}")
    else:
        print(f"  ⚠ could not read robot AE-IDs. If Isaac Sim is not up yet, "
              f"robots get no read permission.")

    pv = [
        {"acor": [config.ORIGINATOR, ae_id], "acop": ACOP_ALL},
        {"acor": [origin_mn(node)],
         "acop": ACOP_RETRIEVE | ACOP_DISCOVERY},
    ]

    # Judge app. Subscribes and receives via notifications.
    subs = config.verdict_subscribers(node)
    if subs:
        acop = ACOP_RETRIEVE | ACOP_NOTIFY | ACOP_DISCOVERY
        if getattr(config, "VERDICT_SUB_CREATE", True):
            acop |= ACOP_CREATE
        pv.append({"acor": subs, "acop": acop})
        print(f"  verdict subscribers: {', '.join(subs)}  (acop={acop})")

    if robots:
        pv.append({"acor": robots,
                   "acop": ACOP_RETRIEVE | ACOP_NOTIFY | ACOP_DISCOVERY})

    acp_ri = om2m.create_acp(
        cse, f"acp-verdict-{node}", pv,
        [{"acor": [config.ORIGINATOR, ae_id], "acop": ACOP_ALL}],
    )
    print(f"  {'✓' if acp_ri else '✗'} acp-verdict-{node}")
    time.sleep(SETTLE)

    om2m.create_container(
        f"{cse}/{ae_name}", config.VERDICT_CNT,
        mni=config.VERDICT_MNI,
        acpi=[acp_ri] if acp_ri else None,
        labels=config.resource_labels(node, "verdict"),
    )
    print(f"  ✓ {ae_name}/{config.VERDICT_CNT}  (mni={config.VERDICT_MNI})")
    time.sleep(SETTLE)


def setup_mn_resources(node: str) -> Dict[str, bool]:
    """
    One site. Bind to the MEP's MN-CSE and
      1) check robot AEs / telemetry (owned by Isaac)
      2) create TRAINING-AE-i (ours)
      3) create cnt-local-model, cnt-net-status, cnt-window-scores
    """
    site = config.SITE_OF_NODE.get(node, node)
    mep = config.MEP_OF_NODE.get(node, "?")
    print(f"\n=== {node} / {site} / {mep} ===")

    cse = bind_mn(node)
    if not reachable(f"{mep} MN-CSE"):
        return {}

    robots = verify_robot_aes(node)

    ae_name = config.training_ae_name(node)
    create_fl_ae(ae_name, origin_mn(node))
    time.sleep(SETTLE)

    acp_ri = setup_mn_acp(node)
    ae = f"{cse}/{ae_name}"

    # Per-round local model cache. With mni=5 only the last 5 rounds are kept.
    om2m.create_container(
        ae, "cnt-local-model", mni=5,
        acpi=[acp_ri] if acp_ri else None,
        labels=config.resource_labels(node, "local-model"),
    )
    print("  ✓ cnt-local-model  (mni=5)")
    time.sleep(SETTLE)

    # MEC028 snapshots and buffer state. Scenarios 2 and 3 write here.
    om2m.create_container(
        ae, "cnt-net-status", mni=100,
        acpi=[acp_ri] if acp_ri else None,
        labels=config.resource_labels(node, "net-status"),
    )
    print("  ✓ cnt-net-status  (mni=100)")
    time.sleep(SETTLE)

    # Per-round window anomaly scores. For dashboard charts; never leaves the site.
    om2m.create_container(
        ae, config.WINDOW_SCORES_CNT, mni=config.WINDOW_SCORES_MNI,
        acpi=[acp_ri] if acp_ri else None,
        labels=config.resource_labels(node, "window-scores"),
    )
    print(f"  ✓ {config.WINDOW_SCORES_CNT}  (mni={config.WINDOW_SCORES_MNI})")
    time.sleep(SETTLE)

    setup_analysis_ae(node, cse)

    return robots


# ════════════════════════════════════════════════════════
# Wait for Isaac Sim
# ════════════════════════════════════════════════════════

def count_ready_robots(node: str) -> tuple:
    """
    Count robots at this site that have everything up to the telemetry container.
    Returns: (names of ready robots, expected count)

    An AE without telemetry is not ready to publish yet, so it is not counted.
    """
    cse = om2m.cse_name()
    site = config.SITE_OF_NODE.get(node, node)
    want = config.ROBOTS_PER_NODE.get(node, 0)

    uril = om2m.discover(cse, ty=2, lim=200)
    names = sorted({
        p.rstrip("/").split("/")[-1]
        for p in uril
        if p.rstrip("/").split("/")[-1].startswith(f"{site}_")
    })
    ready = [n for n in names
             if om2m.resource_exists(f"{cse}/{n}/{config.ROBOT_TELEMETRY_CNT}")]
    return ready, want


def wait_for_robots(nodes: List[str],
                    timeout: float,
                    min_per_site: int) -> bool:
    """
    Wait until Isaac Sim brings up the robot AEs and telemetry.

    We do not create robot AEs. Right after the sandbox starts there are only MN-CSEs,
    and Isaac registers the AEs. So starting FL right away leaves no data to train on.

    Even if the expected count is not reached, proceed once each site has min_per_site robots ready.
    There is no reason to block the whole demo because one robot is broken.
    When time runs out, report the current state and carry on — it does not stop.
    """
    print(f"\n=== Waiting for Isaac Sim (max {timeout:.0f}s, "
          f"min {min_per_site} robots per site) ===")

    deadline = time.time() + timeout
    next_log = 0.0

    while True:
        status = {}
        for node in nodes:
            bind_mn(node)
            status[node] = count_ready_robots(node)

        done = all(len(r) >= min_per_site for r, _ in status.values())
        now = time.time()

        if done or now >= deadline:
            for node, (ready, want) in status.items():
                site = config.SITE_OF_NODE.get(node, node)
                mark = "✓" if len(ready) >= min_per_site else "✗"
                print(f"  {mark} {site}: {len(ready)}/{want} robots "
                      f"({', '.join(ready) if ready else 'none'})")
            if done:
                print("  ✓ ready to start training")
                return True
            print(f"\n  ⚠ not all ready within {timeout:.0f}s.")
            print("    Check that Isaac Sim is registering to this sandbox.")
            return False

        if now >= next_log:
            parts = [f"{config.SITE_OF_NODE.get(n, n)} {len(r)}/{w}"
                     for n, (r, w) in status.items()]
            print(f"  ⏳ {' / '.join(parts)}  "
                  f"({deadline - now:.0f}s left)")
            next_log = now + 10.0

        time.sleep(5.0)


# ════════════════════════════════════════════════════════
# Verify
# ════════════════════════════════════════════════════════

def verify(nodes: List[str], scope: str = "all") -> bool:
    print("\n" + "=" * 60)
    print("  Resource check")
    print("=" * 60)

    ok = True

    if scope in ("all", "in"):
        ok &= _verify_in(nodes)
    if scope in ("all", "mn"):
        ok &= _verify_mn(nodes)

    print()
    print("✓ all our resources exist" if ok else "✗ some resources are missing")
    return ok


def _verify_in(nodes: List[str]) -> bool:
    ok = True
    cse = bind_in()
    print(f"\n[IN-CSE] {om2m.base_url()}/{cse}")
    ae = f"{cse}/{config.IN_AE_NAME}"
    targets = [
        ae,
        f"{ae}/cnt-fl-control",
        f"{ae}/cnt-global-model",
        f"{ae}/cnt-local-updates",
        f"{ae}/cnt-fl-inference",
    ] + [f"{ae}/cnt-local-updates/cnt-{n}" for n in nodes]

    for t in targets:
        hit = om2m.resource_exists(t)
        ok &= hit
        print(f"  {'✓' if hit else '✗'} {t.split('/', 1)[-1]}")
    return ok


def _verify_mn(nodes: List[str]) -> bool:
    ok = True
    for node in nodes:
        cse = bind_mn(node)
        site = config.SITE_OF_NODE.get(node, node)
        mep = config.MEP_OF_NODE.get(node, "?")
        print(f"\n[{mep}] {om2m.base_url()}/{cse}  ({site})")

        ae_name = config.training_ae_name(node)
        for t in (f"{cse}/{ae_name}",
                  f"{cse}/{ae_name}/cnt-local-model",
                  f"{cse}/{ae_name}/cnt-net-status",
                  f"{cse}/{ae_name}/{config.WINDOW_SCORES_CNT}"):
            hit = om2m.resource_exists(t)
            ok &= hit
            print(f"  {'✓' if hit else '✗'} {t.split('/', 1)[-1]}")

        # Robots are owned by Isaac, so missing ones do not count as failures. Warning only.
        robots = verify_robot_aes(node)
        if not robots:
            print("    (training cannot run without robot data)")
    return ok


# ════════════════════════════════════════════════════════
# Reset (our resources only)
# ════════════════════════════════════════════════════════

def clean(nodes: List[str], scope: str = "all") -> None:
    """
    Delete only what we created, via oneM2M DELETE.
    Robot AEs are owned by Isaac, so they are not touched.
    """
    print("\n=== Deleting our resources ===")

    if scope in ("all", "in"):
        _clean_in(nodes)
    if scope in ("all", "mn"):
        _clean_mn(nodes)


def _clean_in(nodes: List[str]) -> None:
    cse = bind_in()
    for name in (f"{cse}/{config.IN_AE_NAME}",):
        print(f"  {'✓' if om2m.delete_resource(name) else '✗'} {name}")
    for node in nodes:
        for n in (f"acp-dropbox-{node}",):
            om2m.delete_resource(f"{cse}/{n}")
    for n in ("acp-fl-control", "acp-global-model", "acp-local-updates"):
        om2m.delete_resource(f"{cse}/{n}")
    print("  ✓ IN-CSE ACP deleted")


def _clean_mn(nodes: List[str]) -> None:
    for node in nodes:
        cse = bind_mn(node)
        ae_name = config.training_ae_name(node)
        print(f"  {'✓' if om2m.delete_resource(f'{cse}/{ae_name}') else '✗'} "
              f"{config.MEP_OF_NODE.get(node)}/{ae_name}")
        om2m.delete_resource(f"{cse}/acp-cache-{node}")


# ════════════════════════════════════════════════════════

def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--clean", action="store_true",
                    help="delete our resources and recreate them")
    ap.add_argument("--verify", action="store_true",
                    help="verify only, do not create")
    ap.add_argument("--scope", choices=("all", "in", "mn"), default="all",
                    help="in: IN-CSE side only (Cloud/FL-FedAvg), mn: site MN-CSE side only "
                         "(Edge-ESTIMED-VM/Local-Model), all: both (default)")
    ap.add_argument("--nodes", default=",".join(config.ALL_NODES),
                    help="target nodes (default: mn1,mn2,mn3)")
    ap.add_argument("--wait-robots", nargs="?", type=float,
                    const=float(os.getenv("FL_ROBOT_WAIT_SEC", "300")),
                    default=None,
                    help="wait until Isaac Sim registers robot AEs (seconds)")
    ap.add_argument("--min-robots", type=int,
                    default=int(os.getenv("FL_MIN_ROBOTS_PER_SITE", "1")),
                    help="min robots per site (default 1)")
    args = ap.parse_args()
    scope = args.scope
    do_in = scope in ("all", "in")
    do_mn = scope in ("all", "mn")

    # When handling only the IN side, no sandbox address is needed (the IN-CSE is outside the sandbox).
    if do_mn:
        config.require_sandbox()

    nodes = [n.strip() for n in args.nodes.split(",") if n.strip()]

    print("=" * 60)
    print("  oneM2M FL resource setup (ACME / one CSE per MEP)")
    print("=" * 60)
    print(f"  Scope    : {scope}")
    if do_mn:
        print(f"  Sandbox  : {config.SANDBOX_HOST} / {config.SANDBOX_NAME}")
    print(f"  Nodes    : {', '.join(nodes)}")
    print(f"  Training : {', '.join(config.TRAINING_NODES)}")
    print(f"  Non-contributing: {', '.join(config.NON_CONTRIBUTING_NODES)}")

    # First check that every CSE to be handled is up. If even one is not, creation is left
    # half done, which is even more confusing.
    print("\n=== CSE connectivity check ===")
    alive = True
    if do_in:
        bind_in()
        alive &= reachable("IN-CSE")
    if do_mn:
        for node in nodes:
            bind_mn(node)
            alive &= reachable(f"{config.MEP_OF_NODE.get(node)} MN-CSE")

    if not alive:
        print("\n✗ some CSEs are unreachable. Check that the sandbox scenario is up")
        print("  and that FL_SANDBOX_HOST / FL_SANDBOX_NAME / FL_IN_BASE_URL are correct.")
        sys.exit(1)

    if args.wait_robots is not None:
        if not do_mn:
            print("✗ --wait-robots only applies to MN scope (--scope mn or all).")
            sys.exit(2)
        ok = wait_for_robots(nodes, args.wait_robots, args.min_robots)
        if args.verify:
            sys.exit(0 if (verify(nodes, scope) and ok) else 1)
        sys.exit(0 if ok else 1)

    if args.verify:
        sys.exit(0 if verify(nodes, scope) else 1)

    if args.clean:
        clean(nodes, scope)
        time.sleep(1.0)

    if do_in:
        acps = setup_in_acps(nodes)
        setup_in_resources(nodes, acps)
    if do_mn:
        for node in nodes:
            setup_mn_resources(node)

    ok = verify(nodes, scope)
    sys.exit(0 if ok else 1)

if __name__ == "__main__":
    main()
