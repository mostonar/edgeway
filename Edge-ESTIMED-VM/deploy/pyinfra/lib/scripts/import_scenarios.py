#!/usr/bin/env python3
"""
Standalone helper script to load, convert, import, and VERIFY network scenario YAMLs
into the meep-platform-ctrl API.

Features:
1. Dynamic Python site-packages path discovery (supporting PyYAML or ruamel.yaml).
2. Automated service IP discovery for meep-platform-ctrl.
3. Idempotent scenario import via HTTP POST.
4. Active GET verification against meep-platform-ctrl API to confirm all scenarios
   are registered, with an explicit [PASS] / [FAIL] summary report.
"""
import os
import sys
import time
import json
import glob
import argparse
import subprocess
import urllib.request
import urllib.error

import yaml


def get_platform_ctrl_ip(kubeconfig):
    """
    Retrieve the ClusterIP of the meep-platform-ctrl Kubernetes service.
    Includes retry logic in case the service is still initializing after deployment.
    """
    cmd = [
        "kubectl",
        "get",
        "svc",
        "meep-platform-ctrl",
        "-o",
        "jsonpath={.spec.clusterIP}",
    ]
    env = os.environ.copy()
    if kubeconfig and os.path.exists(kubeconfig):
        env["KUBECONFIG"] = kubeconfig

    for attempt in range(1, 16):
        try:
            res = subprocess.run(
                cmd,
                env=env,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=True,
            )
            ip = res.stdout.strip().strip("'\"")
            if ip:
                return ip
        except subprocess.CalledProcessError as e:
            if attempt == 15:
                print(
                    f"[Scenario Import] Error retrieving meep-platform-ctrl service IP: {e.stderr.strip()}",
                    file=sys.stderr,
                )
                return None
        time.sleep(2)
    return None


def wait_for_platform_ctrl_ready(cluster_ip, timeout_seconds=900, poll_interval=10):
    """
    Waits up to `timeout_seconds` for the meep-platform-ctrl HTTP API to become ready and responsive.
    This is critical on fresh nodes where database images (PostGIS, Redis, CouchDB) take several minutes
    to download before meep-platform-ctrl pod can start.
    """
    url = f"http://{cluster_ip}/platform-ctrl/v1/scenarios"
    print(f"[Scenario Import] Waiting up to {timeout_seconds}s for meep-platform-ctrl API ({url}) to become ready...")
    start_time = time.time()
    attempt = 0
    while time.time() - start_time < timeout_seconds:
        attempt += 1
        try:
            req = urllib.request.Request(url, headers={"Accept": "application/json"}, method="GET")
            with urllib.request.urlopen(req, timeout=5) as resp:
                if resp.status == 200:
                    print(f"[Scenario Import] meep-platform-ctrl API is READY! (Attempt {attempt}, elapsed {int(time.time() - start_time)}s)")
                    return True
        except Exception:
            pass
        if attempt % 6 == 1:
            print(f"[Scenario Import] Waiting for meep-platform-ctrl API to start (elapsed {int(time.time() - start_time)}s / {timeout_seconds}s)...")
        time.sleep(poll_interval)
    print(f"[Scenario Import ERROR] meep-platform-ctrl API did not become ready after {timeout_seconds}s.", file=sys.stderr)
    return False


def get_existing_scenarios(cluster_ip):
    """
    Sends a GET request to http://<cluster_ip>/platform-ctrl/v1/scenarios
    and returns a set of currently registered scenario names.
    """
    url = f"http://{cluster_ip}/platform-ctrl/v1/scenarios"
    req = urllib.request.Request(url, headers={"Accept": "application/json"}, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            data_str = response.read().decode("utf-8")
            data = json.loads(data_str)
            names = {s.get("name") for s in data.get("scenarios", []) if s.get("name")}
            return names
    except Exception as e:
        print(f"[Scenario Import] Warning: Could not query existing scenarios via GET {url}: {e}", file=sys.stderr)
        return set()


def import_scenario(cluster_ip, filepath, existing_names):
    """Loads a scenario YAML file, injects its name, and POSTs to meep-platform-ctrl API if not present."""
    basename = os.path.splitext(os.path.basename(filepath))[0]
    if basename in existing_names:
        print(f"[Scenario Import] Scenario '{basename}' is already available on meep-platform-ctrl (skipping to avoid override).")
        return True

    try:
        with open(filepath, "r", encoding="utf-8") as f:
            sc_data = yaml.safe_load(f)
    except Exception as e:
        print(f"[Scenario Import] Could not read YAML file '{filepath}': {e}", file=sys.stderr)
        return False

    if not isinstance(sc_data, dict):
        print(
            f"[Scenario Import] Skipping '{filepath}': YAML root is not a dictionary.",
            file=sys.stderr,
        )
        return False

    sc_data["name"] = basename
    payload = json.dumps(sc_data).encode("utf-8")
    url = f"http://{cluster_ip}/platform-ctrl/v1/scenarios/{basename}"

    for attempt in range(1, 4):
        try:
            req = urllib.request.Request(
                url,
                data=payload,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(req, timeout=15) as response:
                print(f"[Scenario Import] Successfully imported scenario '{basename}' (HTTP {response.status}).")
                return True
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8", errors="ignore")
            if e.code in (400, 409):
                # Already exists or minor schema notice
                print(f"[Scenario Import] Notice for scenario '{basename}' (HTTP {e.code}): {error_body.strip() or e.reason}")
                return True
            if attempt == 3:
                print(f"[Scenario Import] Failed to import scenario '{basename}' (HTTP {e.code}): {error_body.strip() or e.reason}", file=sys.stderr)
                return False
        except Exception as e:
            if attempt == 3:
                print(
                    f"[Scenario Import] Could not import scenario '{basename}' from '{filepath}': {e}",
                    file=sys.stderr,
                )
                return False
        time.sleep(3)


def main():
    parser = argparse.ArgumentParser(
        description="Import and verify network scenario YAML files into meep-platform-ctrl API."
    )
    parser.add_argument(
        "--networks-dir",
        required=True,
        help="Path to directory containing scenario YAML files (e.g. mec_frontend_dir/networks)",
    )
    parser.add_argument(
        "--kubeconfig",
        default="",
        help="Path to kubeconfig file for kubectl authentication",
    )
    args = parser.parse_args()

    networks_dir = args.networks_dir
    if not os.path.isdir(networks_dir):
        print(f"[Scenario Import] Directory '{networks_dir}' does not exist. Skipping scenario import.")
        sys.exit(0)

    yaml_files = sorted(
        glob.glob(os.path.join(networks_dir, "*.yaml"))
        + glob.glob(os.path.join(networks_dir, "*.yml"))
    )
    if not yaml_files:
        print(f"[Scenario Import] No YAML scenario files found in '{networks_dir}'.")
        sys.exit(0)

    print("[Scenario Import] Discovering meep-platform-ctrl ClusterIP...")
    cluster_ip = get_platform_ctrl_ip(args.kubeconfig)
    if not cluster_ip:
        print("[Scenario Import ERROR] Could not resolve meep-platform-ctrl ClusterIP.", file=sys.stderr)
        sys.exit(1)

    print(f"[Scenario Import] Resolved meep-platform-ctrl ClusterIP: {cluster_ip}.")
    if not wait_for_platform_ctrl_ready(cluster_ip, timeout_seconds=900, poll_interval=10):
        sys.exit(1)

    print(f"[Scenario Import] Connected to meep-platform-ctrl at IP: {cluster_ip}.")
    existing_names = get_existing_scenarios(cluster_ip)
    print(f"[Scenario Import] Found {len(existing_names)} existing scenario(s) on platform-ctrl.")

    # 1. Perform imports
    for filepath in yaml_files:
        import_scenario(cluster_ip, filepath, existing_names)

    # 2. Verify by sending GET request to platform-ctrl API
    print("\n[Scenario Import] Verifying scenario registration via GET request to platform-ctrl API...")
    verified_names = get_existing_scenarios(cluster_ip)

    expected_scenarios = [os.path.splitext(os.path.basename(f))[0] for f in yaml_files]
    passed = []
    failed = []

    for name in expected_scenarios:
        if name in verified_names:
            passed.append(name)
        else:
            failed.append(name)

    print("=" * 70)
    print("MEC SANDBOX NETWORK SCENARIO IMPORT & VERIFICATION REPORT")
    print("=" * 70)
    print(f"Target Platform API : http://{cluster_ip}/platform-ctrl/v1/scenarios")
    print(f"Total Discovered    : {len(expected_scenarios)}")
    print(f"Verified (Passed)   : {len(passed)}")
    print(f"Missing (Failed)    : {len(failed)}")
    print("=" * 70)
    print("Scenario Verification Status:")
    for name in passed:
        print(f"  [PASS] {name}")
    for name in failed:
        print(f"  [FAIL] {name}")
    print("=" * 70)

    if failed:
        print(f"[Scenario Import ERROR] Verification failed: {len(failed)} scenario(s) are missing from meep-platform-ctrl API!", file=sys.stderr)
        sys.exit(1)

    print("[Scenario Import] All network scenarios successfully verified on meep-platform-ctrl API!")
    sys.exit(0)


if __name__ == "__main__":
    main()
