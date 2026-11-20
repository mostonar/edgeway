# EdgeWay

**Multi-Robot Floor Anomaly Detection and Operation Management with oneM2M and ETSI MEC**

Warehouse AMRs (Autonomous Mobile Robots) drive the same aisles many times a day while moving goods. EdgeWay uses the driving signals from that everyday work to find floor and robot problems. Three sites (A, B, C) learn normal driving together with Federated Learning (FL), while the raw driving data stays on site. When one robot reports an anomaly, another robot is sent over the same spot to decide whether the floor or the robot should be inspected first.

All data and commands between robots and applications go through oneM2M CSEs. Each site's MN-CSE and MEC services run in the ETSI MEC Sandbox.

This repository holds all the code used in the demo: the simulation, the MEC environment, the site apps, FL and the dashboards. This page explains what each module does, how they interact and in which order to run them. Installation and detailed settings for each module are in that module's README.

## Contents

- [System overview](#system-overview)
- [Modules](#modules)
- [oneM2M resources](#onem2m-resources)
- [How it works](#how-it-works)
- [Running](#running)

## System overview

```
[Isaac Sim PC]
  Isaac Sim ── ROS 2 ── Nav2 / demo_task_executor / ROS-IPE   (per robot)
                                         │ WebSocket, HTTP(S)
                                         ▼
[MEC Sandbox VM]  per site A, B, C
  MEP ── ACME MN-CSE, MEC 011 / 013 / 028 / 033, APs, robot terminals
  Local-Model (site app, judge app), Site-Dashboard  ── connect to the MN-CSE
                                         │ FL round commands, weights, global model
                                         ▼
[Cloud]
  IN-CSE ── FL-FedAvg (IN-AE)

FL-Dashboard: reads the IN-CSE and every site MN-CSE
```

The demo uses two PCs and an external IN-CSE:

- **Isaac Sim PC**: simulates three warehouses and eight robots. Each robot runs its own Nav2, task script and IPE.
- **MEC Sandbox VM**: runs the per-site MEC environment and wireless network. The site apps and the Site-Dashboard also run here.
- **IN-CSE**: a shared CSE outside the sandbox. Every site MN-CSE registers with it, and FL-FedAvg and the site apps exchange round commands and models through it.

**Edge and cloud**

- **Site edge**: turns robot observations into driving commands. IPEs upload driving data to the site MN-CSE, and the site app, judge app and Site-Dashboard on the same MN-CSE use it. A robot that moves to another AP still goes through the same site MN-CSE.
- **Cloud**: only handles the model shared by the three sites. Raw driving data, anomaly scores and verdicts stay on each site's MN-CSE; only weights and summary numbers (loss, sample count) reach the IN-CSE.

**Naming**

| Item | Rule |
|---|---|
| Sites | A, B, C; site number i = 1, 2, 3 |
| Robots | 4 at site A, 2 each at sites B and C |
| Robot AE | `siteC_AMR_1` and so on. Site C's robots are called C1 and C2 below. |

## Modules

| Folder | Runs on | Role |
|---|---|---|
| `Field-Isaac-Sim/` | Isaac Sim PC | Simulation, robot control, IPE |
| `Edge-ESTIMED-VM/` | MEC Sandbox VM | ETSI MEC Sandbox and the EdgeWay network scenario |
| `Edge-ESTIMED-VM/Local-Model/` | MEC Sandbox VM | Per-site site app and judge app |
| `Edge-ESTIMED-VM/Site-Dashboard/` | MEC Sandbox VM | Site operations dashboard |
| `Cloud/FL-FedAvg/` | Cloud | FL coordinator |
| `FL-Dashboard/` | Admin host | FL dashboard |

### Field-Isaac-Sim: simulation and robot control

Simulates three warehouses and eight robots in Isaac Sim. Each robot has its own ROS 2 domain (1–8).

| Part | Role |
|---|---|
| `isaacsim/` | Warehouse scene. Floor friction differs per site. |
| `isaac_script/` | Scripts for the Isaac Script Editor |
| `Nav2/` | Per-robot Nav2 launcher and site maps. Guides robots to pickup, drop-off and inspection spots. |
| `demo_task_executor/` | Per-robot task scenarios. They pick pickup and drop-off spots and change the robot's behavior according to the judge app's verdicts. There are FL data collection runs and site C operation runs (floor anomaly, robot anomaly). |
| `ROS-IPE/` | Per-robot IPE that connects ROS 2 and oneM2M, described below |
| `run_all_demo.sh` | Starts the FL data collection run in one go |

The scripts in `isaac_script/`:

- create and remove anomaly conditions such as floor bumps and lobes on wheels or rollers;
- change the cargo mass;
- pass the task scripts' requests to Isaac (`sim_watcher.py`).

**ROS-IPE**:

- Reads three IMUs and the joint states (38 channels) from ROS 2. Aligns the sensor frames and unwraps joint angles that reset every turn into continuous values.
- Packs one second of simulation time into one CIN in `telemetry`, keeping the time and position of every sample. Task state and position go to `status`.
- Receives verdicts and commands from `command` through a subscription and a pollingChannel (PCH) and passes them to ROS 2.
- Buffers data while the MN-CSE cannot be reached and sends it again afterwards. If needed, finds another MN-CSE through MEC 033 and moves there.

Installation and running: [Field-Isaac-Sim/README.md](Field-Isaac-Sim/README.md)

### Edge-ESTIMED-VM: ETSI MEC Sandbox

An ETSI MEC Sandbox based on AdvantEDGE. The EdgeWay network scenario is `etsi-mec-sandbox-frontend/networks/edgeway.yaml`. For each site it brings up:

- one MEP (`mep-a`, `mep-b`, `mep-c`);
- an ACME MN-CSE;
- MEC services: MEC 011 (application enablement), 013 (Location), 028 (WLAN Information), 033 (IoT) and others;
- several Wi-Fi APs and the robot terminals (`robot-c1`, ...);
- entries for the site and judge apps (`edgeway-fl-a/b/c`, `edgeway-judge-c`). The apps themselves run on the VM as external apps.

The ACME MN-CSE adds two features:

- **WebSocket binding** (`ENABLE_WS`): IPEs can connect over WebSocket.
- **MEC integration** (`MEC_ENABLE`): at startup it reports ready to the MEP through MEC 011 and registers as an IoT platform with the MEC 033 IoT service. Other apps can then look up the CSE ID and address through the MEC 033 IoT API.

Each MN-CSE registers with the external IN-CSE.

Installation and running: [Edge-ESTIMED-VM/scripts/README.md](Edge-ESTIMED-VM/scripts/README.md). WebSocket settings: [charts/onem2m-iot/meep-acme-mn-cse/README.md](Edge-ESTIMED-VM/charts/onem2m-iot/meep-acme-mn-cse/README.md)

### Local-Model: site app and judge app

One of each runs per site, next to the site MN-CSE.

**site app** (`site_app/`, `TRAINING-AE-i` and `ANALYSIS-AE-i`)

- Subscribes to its robots' `telemetry`, cuts it into driving windows, scores each window with the previous model and writes the scores to `cnt-window-scores`. When one robot's score exceeds the threshold three times in a row, it posts an anomaly event to `ANALYSIS-AE-i/cnt-verdict`.
- Trains locally on its own site's data every FL round and uploads the result to its drop-box on the IN-CSE.
- The model is a Conditional Variational Autoencoder (CVAE) with 1D CNNs. Given the sensor values together with the driving mode, speed and load, it learns to reconstruct normal driving signals. The further the input is from the reconstruction, the higher the anomaly score.
- It is also a MEC application:
  - finds its MN-CSE through MEC 033 and reports ready through MEC 011;
  - every round, reads each robot's zone and access point through MEC 013 and its link state and RSSI through MEC 028, and writes them to `TRAINING-AE-i/cnt-net-status`;
  - if the MN-CSE stops answering, finds another one through MEC 033 and moves there.

**judge app** (`judge_ae/`, `JUDGE-AE-i`)

- Subscribes to `cnt-verdict`, runs the cross-check and decides between a floor anomaly and a robot anomaly.
- Writes verdicts to the robots' `command` and no-go zones to `cnt-judgement`. Takes the operator's zone-clear requests from `cnt-request`.
- Finds the robots to command by the `siteS:command` label.
- Checks the MN-CSE regularly. While it is down, writes are held and sent again later. It can also find another MN-CSE through MEC 033 and move there.

Installation and running: [Edge-ESTIMED-VM/Local-Model/README.md](Edge-ESTIMED-VM/Local-Model/README.md)

### Site-Dashboard: site operations

Reads robot state and verdicts from the site MN-CSE.

| View | Shows |
|---|---|
| Floor Plan | Robot positions and goals, suspected anomaly spots, no-go zones set by the judge app (red) |
| Factory Overview | Each robot's task state (Picking up, Cross-checking, Robot fault, ...) |
| Event Log | Pickup, travel and drop-off records, cross-checks, inspection waits, connection outage records from the IPEs |

- Operators can ask to clear a no-go zone from the dashboard. The request goes to `JUDGE-AE-i/cnt-request`.
- The server uses only the Python standard library. A collector (`watch_isaac_cse.py`) reads each robot's `status`, `telemetry` and `log` and stores them in SQLite.

Installation and running: [Edge-ESTIMED-VM/Site-Dashboard/README.md](Edge-ESTIMED-VM/Site-Dashboard/README.md)

### Cloud/FL-FedAvg: FL coordinator

An `IN-AE` that talks only to the IN-CSE. Every round it:

1. posts the round command to `cnt-fl-control`;
2. collects the results from the per-site drop-boxes;
3. builds the global model with FedAvg and writes it to `cnt-global-model`;
4. scores it on the fixed validation set and writes the result to `cnt-fl-inference`.

Installation and running: [Cloud/FL-FedAvg/README.md](Cloud/FL-FedAvg/README.md)

### FL-Dashboard: FL view

Shows:

- round progress and per-site loss;
- per-robot anomaly scores against each site's threshold;
- global model evaluation per round;
- site C's local-only training and MN-CSE handovers.

Round results come from the IN-CSE and anomaly scores from each site MN-CSE, so it runs on an admin host that can reach both.

Installation and running: [FL-Dashboard/README.md](FL-Dashboard/README.md)

## oneM2M resources

```
Site MN-CSE (S = A, B, C; i = 1, 2, 3)
├─ siteS_AMR_r                    IPE (one per robot)
│  ├─ telemetry                   one-second sensor batches
│  ├─ status                      task state and position
│  ├─ command                     verdicts and driving commands
│  │  └─ sub_ipe_cmd              subscription → notifications to the IPE's PCH
│  ├─ log                         connection outage records
│  └─ pch                         pollingChannel
├─ TRAINING-AE-i                  site app
│  ├─ cnt-window-scores           per-window anomaly scores
│  └─ cnt-net-status              robot network state (MEC 013, 028)
├─ ANALYSIS-AE-i/cnt-verdict      anomaly events
└─ JUDGE-AE-i                     judge app
   ├─ cnt-judgement               no-go zones
   └─ cnt-request                 operator zone-clear requests

IN-CSE
├─ IN-AE                          FL-FedAvg
│  ├─ cnt-fl-control              round commands
│  ├─ cnt-local-updates
│  │  └─ cnt-mn1, cnt-mn2, cnt-mn3  per-site drop-boxes
│  ├─ cnt-global-model
│  └─ cnt-fl-inference            fixed validation set results
└─ TRAINING-AE-1..3               site apps (receive round commands)
```

**Protocols**

- **IPE**: sends sensor data over WebSocket, so batches keep flowing over one open connection instead of a new HTTP request each. Driving commands are awaited on the PCH with a separate HTTP(S) request, so uploads continue while the IPE waits for commands.
- **Other apps** (site app, judge app, FL-FedAvg, Site-Dashboard): send HTTP requests to their configured CSE. Where the CSE cannot reach the app directly, the app receives notifications through a PCH. FL round commands and drop-box submissions are signaled this way as well.

**Access control**

Each container has its own accessControlPolicy (ACP) that grants access only to the apps that need it.

| Resource | Access |
|---|---|
| Robot `telemetry` | site app subscribes and reads; judge app reads only |
| Robot `command` | only the judge app can write |
| Robot `log` | only that robot's IPE |
| `cnt-global-model` | only FL-FedAvg updates it |

**Discovery by label**

- Robot containers carry a label that combines the site and the container type, such as `siteC:telemetry` and `siteC:command`.
- When a robot is added, the site app finds it with `siteS:telemetry` at startup and every round and subscribes; the judge app finds where to send commands with `siteS:command`.

## How it works

### 1. Collecting driving data

1. A task script picks a pickup or drop-off spot, and Nav2 guides the robot there.
2. The IPE uploads one-second sensor batches to `telemetry` and the task state to `status`.
3. The site app is notified of new `telemetry` and scores each window.
4. The Site-Dashboard reads `status` and shows positions and task states.

### 2. FL round

1. FL-FedAvg posts a round command to `cnt-fl-control`. The site apps are notified through their PCH on the IN-CSE.
2. Each site app trains on its own site's data and uploads the result to its drop-box. In the demo, sites A and B uploaded weights and summary numbers, and site C uploaded summary numbers only.
3. FL-FedAvg builds the global model with FedAvg, writes it to `cnt-global-model` and writes the fixed validation set results to `cnt-fl-inference`.
4. The site apps use the global model in the next round.
5. Every round, each site app reads its robots' network state through MEC 013 and 028 and writes it to `cnt-net-status`.

### 3. Anomaly report and cross-check

1. When a robot's (C2) score exceeds the threshold three times in a row, the site app posts an event to `cnt-verdict` after local training finishes.
2. The judge app receives the event and writes a verdict with the suspected location to the `command` of every robot on the site.
3. Each IPE passes the verdict to ROS 2:
   - C2 moves aside and waits;
   - C1 drives over the spot;
   - the Site-Dashboard shows C1 as Cross-checking.
4. If C1 also reports an anomaly within 30 seconds after reaching the spot, the judge app picks the **floor** for inspection:
   - it writes a no-go zone around both locations to `cnt-judgement`;
   - the Site-Dashboard Floor Plan shows the zone in red, and the robots keep working outside it;
   - when the operator clears the zone, the request goes through `cnt-request` and the judge app tells the robots.
5. If no further report arrives, the judge app picks the **robot that reported first**. C2 stays in inspection wait (`inspection_wait`) and C1 finishes the delivery.

### 4. AP outage and recovery

1. While a robot cannot reach the MN-CSE, its IPE keeps the data in a memory buffer.
2. When the wireless link is back, the IPE sends the buffered items to the same MN-CSE, oldest first, and removes each item once it is delivered.
3. If the link is back but the MN-CSE does not answer, the IPE finds another MN-CSE of the same site through the MEC 033 IoT API:
   - it creates its AE, containers and ACPs there again and resumes sending telemetry;
   - it also creates its PCH and command subscription again;
   - the site app, judge app and Site-Dashboard follow to the new MN-CSE the same way.
4. When the backlog is sent, the IPE writes the outage duration, the number of buffered items and the number of lost items to `log`. The Site-Dashboard shows this record in its Event Log.

## Running

### Requirements

| Host | Needs |
|---|---|
| Isaac Sim PC | Ubuntu 24.04, NVIDIA GPU, Isaac Sim 6.1, Miniforge. ROS 2 and Nav2 are installed with conda. |
| MEC Sandbox VM | An environment that can build and run the ETSI MEC Sandbox (`Edge-ESTIMED-VM`). Python 3 for Local-Model and the Site-Dashboard. |
| IN-CSE | A shared CSE that the site MN-CSEs register with |

Addresses and credentials are kept in local settings files, not in the repository. Each folder has an example.

| File | Module |
|---|---|
| `Field-Isaac-Sim/ROS-IPE/fleet.yaml` | IPE |
| `.env.local` | Local-Model, FL-FedAvg, FL-Dashboard |
| `Edge-ESTIMED-VM/Site-Dashboard/config.json` | Site-Dashboard |

### Start order

1. **MEC Sandbox** (VM)

   Start the sandbox with the `edgeway` scenario (see [Edge-ESTIMED-VM](Edge-ESTIMED-VM)). Each site gets its MEP, MN-CSE, MEC services, APs and robot terminals, and each MN-CSE registers with MEC 033 and the IN-CSE.

2. **Isaac Sim** (Isaac Sim PC)

   Connect with the streaming client and open `isaacsim/testIsaacScene.usd`. Prepare the anomaly conditions for the scenario in the Script Editor, press Play and run `sim_watcher.py`.
   ```bash
   cd Field-Isaac-Sim
   ./env/run_isaac.sh
   ```

3. **Robots** (Isaac Sim PC, ROS terminal)

   Start Nav2, the IPEs and the task scripts for the scenario (see the table below). The IPEs create the robot AEs and containers on the site MN-CSE.

4. **Site apps and judge apps** (VM)

   Checks and creates the MN-CSE resources, waits for the robots, then starts the site app and judge app of all three sites.
   ```bash
   bash Edge-ESTIMED-VM/Local-Model/run_local_model.sh
   ```

5. **FL-FedAvg** (cloud)

   Checks and creates the IN-CSE resources and starts the rounds once the site apps have subscribed to the round commands.
   ```bash
   bash Cloud/FL-FedAvg/run_fedavg.sh
   ```

6. **Dashboards** (any time)
   ```bash
   bash FL-Dashboard/run_dashboard.sh                                               # http://<host>:7000
   cd Edge-ESTIMED-VM/Site-Dashboard && python3 server.py --config config.json      # http://<host>:8090
   python3 watch_isaac_cse.py --config config.json --site C --robots siteC_AMR_1 siteC_AMR_2
   ```

Steps 4 and 5 can run in either order; each waits for the other.

### Scenarios

| Scenario | Robot side (`Field-Isaac-Sim/`) | Where to look |
|---|---|---|
| FL training (all 8 robots, A, B, C) | `./run_all_demo.sh` | FL-Dashboard |
| Floor anomaly (site C) | Create the bumps with `create_floor_bump.py`, start Nav2 and IPE for domains 7 and 8, run `demo_c1_anno_floor.py` and `demo_c2_anno_floor.py` | Site-Dashboard Floor Plan, Event Log |
| Robot anomaly (site C) | Remove the bumps, then the same with `demo_c1_anno_robot.py` and `demo_c2_anno_robot.py` | Site-Dashboard Event Log (`inspection_wait`) |
| AP outage | Take down the AP a robot is attached to in the sandbox, then restore it | Site-Dashboard Event Log, `cnt-net-status` |

Example for a site C scenario:
```bash
./Nav2/run_nav2.sh 7 8
./ROS-IPE/run_ipe.sh 7 8
ROS_DOMAIN_ID=7 python -u demo_task_executor/demo_c1_anno_floor.py
ROS_DOMAIN_ID=8 python -u demo_task_executor/demo_c2_anno_floor.py
```

### Ports

| Port | Use |
|---|---|
| 5001–5003 | Site apps A, B, C. The MN-CSE notifies them directly, so they must be reachable from the sandbox. |
| 6000 | FL-FedAvg notification server. IN-CSE notifications arrive through a PCH, so it does not need to be exposed. |
| 7000 | FL-Dashboard |
| 8090 | Site-Dashboard |
