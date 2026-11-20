# judge_ae

Anomaly judgment AE for EdgeWay. It takes anomaly reports from the detection AE, has another robot of the same site cross-check the reported spot, and decides whether the cause is the **floor** or the **robot**. Verdicts go to the robots through their oneM2M command containers.

Run one process per site (`--site A|B|C`). Each site has its own CSE. In the names below, `<n>` is the site number:

| `--site` | AE (`<n>`) | Detection AE | Robots |
|---|---|---|---|
| `A` | `JUDGE-AE-1` | `ANALYSIS-AE-1` | `siteA_AMR_1` – `siteA_AMR_4` |
| `B` | `JUDGE-AE-2` | `ANALYSIS-AE-2` | `siteB_AMR_1`, `siteB_AMR_2` |
| `C` (default) | `JUDGE-AE-3` | `ANALYSIS-AE-3` | `siteC_AMR_1`, `siteC_AMR_2` |

Robot names are the AE names created by the Field-Isaac-Sim IPE. Use `--robots` to override them.

```
detection AE ──ANALYSIS-AE-<n>/cnt-verdict──> judge_ae ──<robot>/command──> IPE ──> robot (Field-Isaac-Sim)
                                                │  ^
                     JUDGE-AE-<n>/cnt-judgement │  │ JUDGE-AE-<n>/cnt-request
                          (no-go zones)         v  │ (zone clear)
                                               dashboard
```

## Files

| File | Contents |
|---|---|
| `judge_ae.py` | The judgment AE |
| `retrieve_verdict.sh` | Debug helper: polls a container (default `ANALYSIS-AE-<n>/cnt-verdict`) and prints new ANOMALY reports |

## How it judges

1. An `ANOMALY` report for one robot (the target) opens a case: verdict `suspect` to all robots of the site.
2. The other robots (the verifiers) are watched through their `telemetry`. When one passes within 3 m of the reported spot:
   - if a verifier also reports an anomaly near the spot → **final: floor**. A no-go zone around both spots is published to `cnt-judgement` and sent with the verdict.
   - if no report arrives within `--grace-s` (30 s) after a verifier passed → **final: robot**. Reports from that robot are ignored until a `clear_fault` command appears in its command container.
3. A case with no conclusion within `--timeout-s` (600 s) is closed. Only one case is open at a time.
4. A dashboard `zone-clear` request at a point inside a zone removes the zone and sends `stage: closed` to the robots.

## Resources

| Direction | Resource | Content |
|---|---|---|
| subscribes | `ANALYSIS-AE-<n>/cnt-verdict` | `{"type": "amr-verdict", "verdict": "ANOMALY", "site": "siteC", "robot": "CsiteC_AMR_1", "pose": {"x", "y"}}` |
| subscribes | `JUDGE-AE-<n>/cnt-request` | `{"type": "zone-clear", "x": 60.0, "y": 14.0}` |
| reads | `<robot>/telemetry/la` | verifier position (`pose.x`, `pose.y` arrays) |
| reads | `<robot>/command/la` | `{"command": "clear_fault"}` after maintenance |
| writes | `JUDGE-AE-<n>/cnt-judgement` | `{"zones": [{"x1", "y1", "x2", "y2"}, ...]}` |
| writes | `<robot>/command` | `{"type": "verdict", "case_id", "stage": "suspect" \| "final" \| "closed", "site", "target", "pos", "cause": "robot" \| "floor", "zone"}` |

Reports whose `site` is not this site are ignored.

## Access control

The AE registers itself with originator `CJUDGE-AE-<n>` and creates `acp_judgement` (`CJUDGE-AE-<n>` + `CAdmin` only) for its own containers. The other AEs must grant it access:

- Detection AE: `ANALYSIS-AE-<n>/cnt-verdict` must allow `CJUDGE-AE-<n>` to subscribe and retrieve.
- Robot IPEs: `judge_ae: "CJUDGE-AE-<n>"` for the site in `Field-Isaac-Sim/simple_ipe/fleet.yaml` (already set in `fleet.example.yaml`; telemetry RETRIEVE/DISCOVERY, command CREATE/RETRIEVE/DISCOVERY).
- Dashboard: uses `CAdmin`, so it can read `cnt-judgement` and write `cnt-request`.

## Requirements

- Python 3.8 or later
- `pip install requests`
- A oneM2M CSE (e.g. ACME) per site, where that site's robots and detection AE are registered

## Running

```bash
python judge_ae.py --site <A|B|C> --cse <CSE base URL incl. CSE name> [options]
```

How notifications reach the AE:

- **Without `--notify` (recommended):** the AE fetches notifications from the CSE through a pollingChannel. The CSE never connects to the AE, so no port needs to be open.
- **With `--notify http://<this host>:9090`:** the CSE posts notifications to this URL, and the AE listens on `--port` (default 9090). The CSE must be able to reach that address.

Examples:

```bash
# CSE on the same network
python judge_ae.py --site C --cse http://<host>:<port>/<CSE name>

# MEC CSEs through the sandbox HTTPS gateway (self-signed certificate), one terminal per site
python judge_ae.py --site A --cse https://<MEC_HOST>/<SANDBOX>/mep-a/meep-acme-mn-cse/acme-cse-mn-mep-a-<SANDBOX> --insecure
python judge_ae.py --site B --cse https://<MEC_HOST>/<SANDBOX>/mep-b/meep-acme-mn-cse/acme-cse-mn-mep-b-<SANDBOX> --insecure
python judge_ae.py --site C --cse https://<MEC_HOST>/<SANDBOX>/mep-c/meep-acme-mn-cse/acme-cse-mn-mep-c-<SANDBOX> --insecure
```

To run it inside MEC, point `--cse` at the CSE address reachable inside the MEC network and leave out `--notify`. The code is the same in both cases.

| Option | Default | Meaning |
|---|---|---|
| `--site` | C | Site to judge: A, B or C (`siteA` etc. also accepted) |
| `--robots` | all robots of the site | Comma-separated robot AE names |
| `--insecure` / `--ca <file>` | verify | Skip or set the CA for an https CSE with a self-signed certificate |
| `--check-s` | 1 | s, verifier position poll interval |
| `--grace-s` | 30 | s, wait for the verifier's report after it passes the spot |
| `--timeout-s` | 600 | s, maximum case duration |
| `--zone-margin` | 2 | m, no-go zone margin around both spots |
| `--zone-min` | 5 | m, minimum no-go zone width/height |
| `--alive-s` | 5 | s, CSE liveness check interval |
| `--iot` | off | MEP's MEC 033 `registered_iot_platforms` URL, enables moving to another CSE |

Ctrl+C (or SIGTERM) removes the AE's subscriptions before exiting.

### CSE outage and CSE move

Every `--alive-s` seconds the AE retrieves its own AE resource from the CSE.

- **No response (or 5xx):** verdicts and zone updates are held in order and resent when the CSE answers again.
- **CSE answers again:** the AE, containers, pollingChannel and subscriptions are created again if they are missing, then the held writes are sent.
- **With `--iot`:** while the CSE is down, the AE asks the MEP for the current CSE:
  - MEP does not answer: the network is down, so the AE keeps waiting.
  - Same CSE: the AE keeps retrying it.
  - Different CSE: the AE switches to it, registers there and resends the held writes.

The CSE is taken from `customServicesTransportInfo` entries whose `id` is a CSE-ID (`/…`): `endpoint.uris[0]` + `/` + `name`. The MEP reports an internal host, so the scheme and host of the `--iot` URL are used instead.

```bash
python judge_ae.py --site C --cse https://<MEC_HOST>/<SANDBOX>/mep-c/meep-acme-mn-cse/acme-cse-mn-mep-c-<SANDBOX> --insecure \
    --iot https://<MEC_HOST>/<SANDBOX>/mep-c/iots/v1/registered_iot_platforms
```

After a move, `ANALYSIS-AE-<n>/cnt-verdict` and the robot AEs must exist on the new CSE as well. Until they do, the AE keeps retrying the subscriptions.

### retrieve_verdict.sh

```bash
CSE=<CSE base URL incl. CSE name> ./retrieve_verdict.sh [target path] [interval s]
SITE=A CSE=... ./retrieve_verdict.sh                # site A detection AE (default C)
ORIGIN=CJUDGE-AE-3 CSE=... ./retrieve_verdict.sh    # check access as the judgment AE
```

Needs `curl` and `python3`.

## Testing with the simulation

Site C scenarios are in `Field-Isaac-Sim` (see its README): `demo_c1_anno_floor.py` + `demo_c2_anno_floor.py` for a floor anomaly, `demo_c1_anno_robot.py` + `demo_c2_anno_robot.py` for a robot anomaly. Start `judge_ae.py --site C` before the demos. The detection AE must also be running, because it produces the anomaly reports.
