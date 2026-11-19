# judge_ae

Site C anomaly judgment AE (`JUDGE-AE-3`) for EdgeWay. It takes anomaly reports from the detection AE, has the other robot cross-check the reported spot, and decides whether the cause is the **floor** or the **robot**. Verdicts go to the robots through their oneM2M command containers.

```
detection AE ──ANALYSIS-AE-3/cnt-verdict──> judge_ae ──<robot>/command──> IPE ──> robot (isaac_robot)
                                              │  ^
                     JUDGE-AE-3/cnt-judgement │  │ JUDGE-AE-3/cnt-request
                          (no-go zones)       v  │ (zone clear)
                                             dashboard
```

## Files

| File | Contents |
|---|---|
| `judge_ae.py` | The judgment AE |
| `retrieve_verdict.sh` | Debug helper: polls a container (default `ANALYSIS-AE-3/cnt-verdict`) and prints new ANOMALY reports |

## How it judges

1. An `ANOMALY` report for one robot (the target) opens a case: verdict `suspect` to both robots.
2. The other robot (the verifier) is watched through its `telemetry`. When it passes within 3 m of the reported spot:
   - if it also reports an anomaly near the spot → **final: floor**. A no-go zone around both spots is published to `cnt-judgement` and sent with the verdict.
   - if no report arrives within `--grace-s` (30 s) → **final: robot**. Reports from that robot are ignored until a `clear_fault` command appears in its command container.
3. A case with no conclusion within `--timeout-s` (600 s) is closed. Only one case is open at a time.
4. A dashboard `zone-clear` request at a point inside a zone removes the zone and sends `stage: closed` to the robots.

## Resources

| Direction | Resource | Content |
|---|---|---|
| subscribes | `ANALYSIS-AE-3/cnt-verdict` | `{"type": "amr-verdict", "verdict": "ANOMALY", "site": "siteC", "robot": "CsiteC_AMR_1", "pose": {"x", "y"}}` |
| subscribes | `JUDGE-AE-3/cnt-request` | `{"type": "zone-clear", "x": 60.0, "y": 14.0}` |
| reads | `<robot>/telemetry/la` | verifier position (`pose.x`, `pose.y` arrays) |
| reads | `<robot>/command/la` | `{"command": "clear_fault"}` after maintenance |
| writes | `JUDGE-AE-3/cnt-judgement` | `{"zones": [{"x1", "y1", "x2", "y2"}, ...]}` |
| writes | `<robot>/command` | `{"type": "verdict", "case_id", "stage": "suspect" \| "final" \| "closed", "site", "target", "pos", "cause": "robot" \| "floor", "zone"}` |

Robots: `siteC_AMR_1`, `siteC_AMR_2` (the AE names created by the isaac_robot IPE).

## Access control

The AE registers itself with originator `CJUDGE-AE-3` and creates `acp_judgement` (`CJUDGE-AE-3` + `CAdmin` only) for its own containers. The other AEs must grant it access:

- Detection AE: `ANALYSIS-AE-3/cnt-verdict` must allow `CJUDGE-AE-3` to subscribe and retrieve.
- Robot IPEs: set `judge_ae: "CJUDGE-AE-3"` for siteC in `isaac_robot/simple_ipe/fleet.yaml` (telemetry RETRIEVE/DISCOVERY, command CREATE/RETRIEVE/DISCOVERY).
- Dashboard: uses `CAdmin`, so it can read `cnt-judgement` and write `cnt-request`.

## Requirements

- Python 3.8 or later
- `pip install requests`
- A oneM2M CSE (e.g. ACME) where the site C robots and the detection AE are registered

## Running

```bash
python judge_ae.py --cse <CSE base URL incl. CSE name> [options]
```

How notifications reach the AE:

- **Without `--notify` (recommended):** the AE fetches notifications from the CSE through a pollingChannel. The CSE never connects to the AE, so no port needs to be open.
- **With `--notify http://<this host>:9090`:** the CSE posts notifications to this URL, and the AE listens on `--port` (default 9090). The CSE must be able to reach that address.

Examples:

```bash
# CSE on the same network
python judge_ae.py --cse http://<host>:<port>/<CSE name>

# MEC CSE through the sandbox HTTPS gateway (self-signed certificate)
python judge_ae.py --cse https://<MEC_HOST>/<SANDBOX>/mep-c/meep-acme-mn-cse/acme-cse-mn-mep-c-<SANDBOX> --insecure
```

To run it inside MEC, point `--cse` at the CSE address reachable inside the MEC network and leave out `--notify`. The code is the same in both cases.

| Option | Default | Meaning |
|---|---|---|
| `--insecure` / `--ca <file>` | verify | Skip or set the CA for an https CSE with a self-signed certificate |
| `--check-s` | 1 | s, verifier position poll interval |
| `--grace-s` | 30 | s, wait for the verifier's report after it passes the spot |
| `--timeout-s` | 600 | s, maximum case duration |
| `--zone-margin` | 2 | m, no-go zone margin around both spots |
| `--zone-min` | 5 | m, minimum no-go zone width/height |

Ctrl+C (or SIGTERM) removes the AE's subscriptions before exiting.

### retrieve_verdict.sh

```bash
CSE=<CSE base URL incl. CSE name> ./retrieve_verdict.sh [target path] [interval s]
ORIGIN=CJUDGE-AE-3 CSE=... ./retrieve_verdict.sh    # check access as the judgment AE
```

Needs `curl` and `python3`.

## Testing with the simulation

Site C scenarios are in `isaac_robot` (see its README): `demo_c1_anno_floor.py` + `demo_c2_anno_floor.py` for a floor anomaly, `demo_c1_anno_robot.py` + `demo_c2_anno_robot.py` for a robot anomaly. Start `judge_ae.py` before the demos. The detection AE must also be running, because it produces the anomaly reports.
