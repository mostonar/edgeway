# Local-Model — site apps and judge apps

Everything that runs next to the site MN-CSEs:

| Folder / file | What it runs |
| --- | --- |
| `site_app/` | the three FL site apps, one per site (`TRAINING-AE-i` + `ANALYSIS-AE-i`, i = 1, 2, 3 for sites A, B, C) |
| `judge_ae/` | the judge apps (`JUDGE-AE-n`), one per site; floor fault or robot fault |
| `run_local_model.sh` | starts all three site apps and the judge apps |
| `run_site_app.sh A\|B\|C` | starts one site app |
| `run_judge.sh A\|B\|C` | starts one judge app |

Each site app subscribes to its robots' `telemetry` on the site MN-CSE, trains locally
each round and uploads to the IN-CSE drop-box: weights and summary numbers from sites
A and B, summary numbers only from site C. Everything site-level stays on its own
MN-CSE: window anomaly scores in `TRAINING-AE-i/cnt-window-scores` (read by
`FL-Dashboard`) and anomaly verdicts in `ANALYSIS-AE-i/cnt-verdict`. It is also a MEC app: MEC 033 finds its MN-CSE, MEC 011 registers it,
MEC 028 / MEC 013 add link state and location. The judge app of each site subscribes
to that `cnt-verdict`.

## Judge app

`judge_ae/judge_ae.py` (`JUDGE-AE-n`, n = 1, 2, 3 for sites A, B, C) decides whether an
anomaly report points to the floor or to the robot, by sending a second robot over the
same spot.

1. A report in `ANALYSIS-AE-n/cnt-verdict` opens a case. The judge app writes a
   `suspect` verdict with the location to every robot's `command`. The reporting robot
   moves aside and waits; another robot drives over the spot (cross-check).
2. Once the cross-check robot comes within 3 m of the spot, the judge app waits 30 s
   (`--grace-s`) for its report.
3. The second robot reports there too → `final` verdict, cause `floor`. A no-go zone
   around both locations goes to `JUDGE-AE-n/cnt-judgement`, and the robots keep
   working outside it.
4. No report → `final` verdict, cause `robot`. The first robot waits for inspection.

The Site-Dashboard shows the zones and can ask to clear one through
`JUDGE-AE-n/cnt-request` (`{"type": "zone-clear", "x", "y"}`). The judge app then
removes the zone and sends a `closed` verdict to the robots.

| Resource | Use |
| --- | --- |
| `ANALYSIS-AE-n/cnt-verdict` | subscribed: anomaly reports from the site app |
| `JUDGE-AE-n/cnt-request` | subscribed: zone-clear requests from the Site-Dashboard |
| `JUDGE-AE-n/cnt-judgement` | written: current no-go zones |
| `<robot>/command` | written: `suspect` / `final` / `closed` verdicts |
| `JUDGE-AE-n/acp_judgement` | only `JUDGE-AE-n` and `CAdmin` have access |

- **Robots**: found by the label `<site>:command` (e.g. `siteC:command`) that the robot
  IPEs put on their `command` containers, checked again every 30 s. `--robots` fixes the
  list instead.
- **Notifications**: arrive through a `<pollingChannel>` unless `--notify` gives a URL
  the CSE can reach.
- **CSE outage**: the judge app checks the MN-CSE every 5 s (`--alive-s`). While it is
  down, writes are held and sent again in order once it answers. With `--iot` (the
  MEP's MEC 033 `registered_iot_platforms` URL, set by `run_judge.sh`), it asks the MEP
  for the current MN-CSE and, if it moved, creates its AE, containers and subscriptions
  there again.

`judge_ae.py` needs only `requests` and runs on its own; the usage block at the top of
the file lists all options.

## Setup

```bash
pip install -r Edge-ESTIMED-VM/Local-Model/requirements.txt
cp Edge-ESTIMED-VM/Local-Model/env.local.example .env.local   # repository root, or this folder
```

Fill in:

| Variable | Why |
| --- | --- |
| `FL_SANDBOX_HOST`, `FL_SANDBOX_NAME` | the MEC Sandbox; the MN-CSE and MEC addresses are derived from them |
| `FL_IN_BASE_URL`, `FL_IN_CSE_NAME` (+ `FL_IN_USER` / `FL_IN_PASS`) | the shared IN-CSE |
| `FL_NOTIFY_HOST` | this host's IP. The MN-CSEs notify the site apps at `http://<FL_NOTIFY_HOST>:5001-5003`, and they run inside sandbox containers, so `127.0.0.1` does not work |
| `FL_TLS_VERIFY=0` | only if the sandbox uses a self-signed certificate (also passes `--insecure` to the judge apps) |

`.env*` is git-ignored; never commit it.

## Run everything

```bash
bash Edge-ESTIMED-VM/Local-Model/run_local_model.sh      # from the repository root
cd Edge-ESTIMED-VM/Local-Model && bash run_local_model.sh # or from this folder
```

The script works from any directory and only uses files in its own folder, so the
folder also runs on its own when copied elsewhere.

The script:

1. stops previous site apps (and judge apps, if it is going to start them), frees
   ports 5001–5003 and empties `/tmp/fl_models/local` and `/cache`,
2. checks the MN-CSE resources (`site_app/setup_resources_standard.py --scope mn --verify`)
   and creates them if any are missing,
3. waits up to `FL_ROBOT_WAIT_SEC` (300 s) for at least one robot per site from Isaac Sim;
   robots that come up later join from that round,
4. starts the three site apps (`run_site_app.sh A`, `B`, `C`),
5. starts the judge apps (`run_judge.sh`) for `JUDGE_SITES` (default `A,B,C`). Each gets
   `--cse` = its site's MN-CSE and `--iot` = that MEP's MEC 033 URL, and receives
   notifications through a `<pollingChannel>` (no extra port).

Logs go to `Edge-ESTIMED-VM/Local-Model/logs/run_<time>/` (`mn1..3.log`, `judge_A..C.log`).

Start order with `Cloud/FL-FedAvg` does not matter; the coordinator waits for the
site apps' subscriptions. Ctrl+C stops everything this script started.

## Run one at a time

```bash
bash run_site_app.sh A     # site A only: mn1, port 5001
bash run_judge.sh C        # judge app for site C only
```

Run on its own, `run_site_app.sh` does the preparation for that one site: it stops
that site's previous app, checks and creates that site's MN-CSE resources and waits for
its robots. `run_judge.sh` stops a judge app already running for that site first.
Start a judge app after its site's resources exist (`ANALYSIS-AE-n/cnt-verdict`).
Logs go to `logs/run_<time>/` in this folder.

The site argument also accepts `siteA`, `mn1` or `1`.

## Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `JUDGE_SITES` | `A,B,C` | judge apps to start; empty starts none and leaves any running judge alone |
| `JUDGE_AE_DIR` | `judge_ae/` here | where `judge_ae.py` is |
| `FL_ROBOT_WAIT_SEC` | `300` | how long to wait for the robots |
| `FL_MIN_ROBOTS_PER_SITE` | `1` | robots required per site before starting |
| `FL_SENSOR_FROM_CSE` | `1` | `0` trains from local files instead (`AMR_DATA` = telemetry folder); no judge apps |
| `FL_RESET_ON_START` | `1` | `0` keeps the previous run's local models and cache |

The full list of FL settings is in `site_app/config.py`.

## Ports

| Port | Use |
| --- | --- |
| 5001, 5002, 5003 | site apps A, B, C. The site MN-CSE calls them directly, so they must be reachable from the sandbox. The same address goes into the MEC 011 registration (`FL_MEC_APP_HOST`, or `FL_NOTIFY_HOST` if unset). |

## Network scenario 2 and diagnostics

Load `.env.local` into the shell first (`set -a; source .env.local; set +a`), then:

```bash
python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py watch   --node mn3 --seconds 60
python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py s2      --node mn3 --robot siteC_AMR_2
python3 Edge-ESTIMED-VM/Local-Model/site_app/net_scenarios.py restore --node mn3   # always finish with this

python3 Edge-ESTIMED-VM/Local-Model/site_app/mec_iot.py                  # MEC 033: MN-CSE address per site
python3 Edge-ESTIMED-VM/Local-Model/site_app/mec_wai.py --nodes mn3      # MEC 028 link state
python3 Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py          # how full each robot's telemetry is
```

## Files in `site_app/`

| File | Role |
| --- | --- |
| `mn_ae_standard.py` | the site app (training + analysis) |
| `edge_node.py`, `amr_ingest.py`, `verdict.py` | local training and scoring, CIN → windows, verdict writing |
| `mec_iot.py`, `mec_app.py`, `mec_services.py`, `mec_wai.py`, `mec_loc.py`, `mec_topology.py` | MEC 033 / 011 / 028 / 013 clients |
| `cse_failover.py` | MN-CSE handover (scenario 6) |
| `net_scenarios.py`, `mec_sandbox.py` | network scenario 2 and the sandbox controller |
| `setup_resources_standard.py` | creates/checks oneM2M resources; `--scope mn` here |
| `prepare_data_amr.py` | offline mode: telemetry files → training data |
| `check_telemetry.py`, `probe_routing.py` | diagnostics: telemetry fill level, MN-CSE relaying |
| `config.py`, `amr_schema.py`, `onem2m_utils.py`, `onem2m_pch.py`, `model_cvae.py`, `model_codec.py` | shared modules (the same files also live in the Cloud FL folders) |
