#!/usr/bin/env python3
"""
Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py — shows how the robot telemetry containers are filling up.

[Why it is needed]
When per-site collection does not grow past "windows 33/50", logs alone cannot tell
which of three causes it is.

  (1) Robots stopped publishing              -> cni and the last ct do not change
  (2) Container mni is small, so entries are deleted -> cni sticks at mni
  (3) The cursor (cra) query returns nothing new -> new CINs appear but the query returns 0

Looks twice, 15 seconds apart, to tell those three apart.

Usage:
    cd ~/projects/edgeway-FL
    set -a; source .env.local; set +a
    python3 Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py              # all
    python3 Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py mn2          # one site only
"""

import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import config                      # noqa: E402
import onem2m_utils as om2m        # noqa: E402

GAP_SEC = float(os.getenv("CHECK_GAP_SEC", "15"))


def snapshot(node):
    """Current state of the telemetry containers of this site's robots."""
    om2m.set_endpoint(*config.mn_endpoint(node))
    cse = config.mn_endpoint(node)[1]
    out = {}

    for rb in config.robot_names(node):
        path = f"{cse}/{rb}/{config.ROBOT_TELEMETRY_CNT}"
        res = om2m.get_resource(path)
        cnt = (res or {}).get("m2m:cnt")

        if not cnt:
            out[rb] = None
            continue

        last = om2m.get_latest_content_instance(path)
        last_ct = ((last or {}).get("m2m:cin") or {}).get("ct")

        out[rb] = {
            "path": path,
            "mni": cnt.get("mni"),      # Max instances (unlimited if absent)
            "cni": cnt.get("cni"),      # Current count
            "last_ct": last_ct,         # Creation time of the last CIN
        }

    return out


def count_after(path, since_ct):
    """How many CINs the query after the cursor returns. The exact path the MN-AE uses."""
    try:
        uril = om2m.discover_cins_after(path, since_ct, lim=300)
        return len(uril or [])
    except Exception as e:
        return f"query failed: {e}"


def main():
    nodes = sys.argv[1:] or list(config.ALL_NODES)

    print("=" * 66)
    print("  Robot telemetry container check")
    print(f"  checking twice, {GAP_SEC:.0f}s apart")
    print("=" * 66)

    first = {n: snapshot(n) for n in nodes}

    for node in nodes:
        site = config.SITE_OF_NODE.get(node, node)
        print(f"\n[{node} / {site}]  {len(first[node])} robots")
        for rb, s in first[node].items():
            if s is None:
                print(f"  ✗ {rb}: cannot read telemetry container")
                continue
            mni = s["mni"] if s["mni"] is not None else "unlimited"
            print(f"  {rb}: mni={mni}  cni={s['cni']}  last_ct={s['last_ct']}")

    print(f"\nwaiting {GAP_SEC:.0f}s ...")
    time.sleep(GAP_SEC)

    second = {n: snapshot(n) for n in nodes}

    print("\n" + "=" * 66)
    print("  Result")
    print("=" * 66)

    for node in nodes:
        site = config.SITE_OF_NODE.get(node, node)
        print(f"\n[{node} / {site}]")
        om2m.set_endpoint(*config.mn_endpoint(node))

        for rb in first[node]:
            a, b = first[node][rb], second[node].get(rb)
            if a is None or b is None:
                print(f"  ✗ {rb}: cannot read container")
                continue

            # Use the last ct of the first snapshot as the cursor and see how many
            # are returned after it. Same as what the MN-AE does every round.
            after = count_after(a["path"], a["last_ct"])
            grew = (a["last_ct"] != b["last_ct"])

            mni = a["mni"] if a["mni"] is not None else "unlimited"
            print(f"  {rb}")
            print(f"    mni={mni}  cni: {a['cni']} -> {b['cni']}")
            print(f"    last CIN time changed: {'yes' if grew else 'no'}")
            print(f"    returned after cursor: {after}")

            if not grew:
                print("    => robots stopped publishing. Check Isaac Sim.")
            elif isinstance(after, int) and after == 0:
                print("    => new CINs appear but the cra query returns 0. "
                      "The CSE createdAfter filter is the problem.")
            elif (a["mni"] is not None
                  and str(a["cni"]) == str(a["mni"])):
                print("    => cni is at mni. "
                      "Old CINs are deleted as new ones arrive. Raise mni.")
            else:
                print("    => publishing and querying both work. This is fine.")


if __name__ == "__main__":
    main()
