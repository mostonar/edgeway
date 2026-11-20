# Field-Isaac-Sim

The simulation part of EdgeWay. Eight AMRs in an Isaac Sim warehouse scene each drive with their own Nav2 stack, and per-robot IPEs upload sensor data to the oneM2M MN-CSE on MEC.

```
Isaac Sim (scene + sim_watcher)
   │  ROS 2 (one robot = one ROS_DOMAIN_ID)
   ├─ Nav2 (per robot)               Nav2/
   ├─ Demo scripts (task execution)  demo_task_executor/
   └─ IPE (per robot)                ROS-IPE/     ──HTTP/WS──>  oneM2M MN-CSE (MEC)
```

FL training, the dashboard and the judgment AE are not part of this folder.

## Layout

| Folder | Contents |
|---|---|
| `isaacsim/` | Scene `testIsaacScene.usd` |
| `isaac_script/` | Scripts run in the Isaac Script Editor (`sim_watcher.py`, ...) |
| `Nav2/` | Per-robot Nav2 launcher, parameters, site maps |
| `ROS-IPE/` | oneM2M IPE (`ipe_proc.py`) and its config |
| `demo_task_executor/` | Per-robot task scenarios (ROS 2 nodes) |
| `env/` | conda environment specs and environment scripts |
| `run_all_demo.sh` | FL data collection: starts Nav2 -> IPE -> 8 demos in order |

## Robots and domains

| ROS_DOMAIN_ID | Robot (Isaac prim) | Site | Map |
|---|---|---|---|
| 1–4 | `site_a_AMR_1` – `site_a_AMR_4` | A | `Nav2/maps/siteA.yaml` |
| 5–6 | `site_b_AMR_1`, `site_b_AMR_2` | B | `Nav2/maps/siteB.yaml` |
| 7–8 | `site_c_AMR_1`, `site_c_AMR_2` | C | `Nav2/maps/siteC.yaml` |

This mapping lives in three places: `Nav2/run_nav2.sh` (`ROBOTS`), `ROS-IPE/fleet.yaml` (`robots`) and the demo scripts (`ROBOT`). Keep them in sync when changing it.

## Requirements

- Ubuntu 24.04 (x86_64), NVIDIA GPU
- Isaac Sim 6.1 (`~/isaacsim`; tested with `6.1.0-rc.26`). The scene references NVIDIA's public assets (warehouse, forklift robot) by URL, so an internet connection is required.
- Miniforge (`~/miniforge3`). Everything is installed with conda, no sudo needed.
- Isaac Sim WebRTC Streaming Client (on the PC used to view Isaac)
- oneM2M MN-CSE (e.g. ACME), one per site

Other install locations can be set with the `ISAACSIM_DIR`, `CONDA_DIR` and `ROS_TARBALL_DIR` environment variables (see `env/*.sh`).

## Installation

### 0. Isaac Sim, Miniforge

- Isaac Sim 6.1: extract the Linux standalone package to `~/isaacsim`. For another location, set `ISAACSIM_DIR`.
- Miniforge:

  ```bash
  wget https://github.com/conda-forge/miniforge/releases/latest/download/Miniforge3-Linux-x86_64.sh
  bash Miniforge3-Linux-x86_64.sh -b -p ~/miniforge3
  ```

- Isaac Sim WebRTC Streaming Client: install it on the PC used to view Isaac.

### 1. ROS 2 Humble tarball (libraries for the Isaac ROS 2 bridge)

```bash
mkdir -p ~/ros2_humble && cd ~/ros2_humble
wget https://github.com/ros2/ros2/releases/download/release-humble-20260914/ros2-humble-20260914-linux-jammy-amd64.tar.bz2
tar xf ros2-humble-20260914-linux-jammy-amd64.tar.bz2      # -> ~/ros2_humble/ros2-linux
```

**Do not source** this tarball's `setup.bash`. `env/run_isaac.sh` only adds its lib folder.

### 2. Two conda environments

```bash
./env/setup_ros310.sh                    # ros310: old library versions used by the Isaac bridge (not for running anything)
conda env create -f env/ros_nav.yml      # ros_nav: Nav2, IPE, demo scripts
```

`env/ros_nav.lock.yml` is the full export from the test environment (reference only). Use it to compare versions if `ros_nav.yml` does not solve.

### 3. fleet.yaml

```bash
cp ROS-IPE/fleet.example.yaml ROS-IPE/fleet.yaml
```

Replace `<MEC_HOST>` and `<SANDBOX>` in `sites.<site>.cse_url` and `sites.<site>.ws.url` with your CSE addresses. `fleet.yaml` is git-ignored.

Optional per site (see the comments in `fleet.example.yaml`):

- `binding`: `ws` (the example's setting, oneM2M WebSocket at `ws.url`, needs `websocket-client`) or `http`.
- `iot.url`: the MEP's MEC 033 `registered_iot_platforms` URL. Enables moving to another CSE (see [CSE outage](#cse-outage)).

## Running

Two kinds of terminals are used:

- Isaac terminal: `./env/run_isaac.sh`
- ROS terminal: `source env/ros_env.sh` (activates ros_nav). Do not source the ROS tarball `setup.bash` here either.

### 1. Prepare Isaac Sim

1. In the Isaac terminal, run `./env/run_isaac.sh`. Isaac starts as a streaming server without a window (`--no-window`). Wait 1–2 minutes for it to finish loading.
2. On the viewing PC, open the WebRTC Streaming Client and connect to the IP of the PC running Isaac. When connecting from another PC, open the streaming ports (TCP 49100, UDP 47998) in the firewall.
3. Open `isaacsim/testIsaacScene.usd` with File → Open.
4. In **Stop state**, run the preparation needed for your scenario in the Script Editor (Window → Script Editor): paste or open the script, adjust the settings at the top, then Run.

   | Script | When |
   |---|---|
   | `isaac_script/create_floor_bump.py` | Floor anomaly scenario: `REMOVE = False` creates the bumps (x 67.51). Other scenarios: `REMOVE = True` removes them |
   | `isaac_script/make_c1_bump.py` | Capture scenario: `REMOVE = False` creates bumps on c1's lane (x 58.88). Afterwards restore with `create_floor_bump.py` |
   | `isaac_script/make_wheel_lobe.py` | Only to rebuild the c2 wheel lobes: `MODE = "make"` (already in the scene). `MODE = "off"` also works during Play |
   | `isaac_script/robot_anomaly_apply.py` | Only to rebuild the `tire_active_<robot>` materials (already in the scene for all 8 robots) |
   | `isaac_script/apply_materials.py` | To re-apply or check the physics materials (already applied in the scene) |

5. Press Play, then run `isaac_script/sim_watcher.py` once in the Script Editor. The demo scripts send commands (cargo mass, anomaly injection) through `~/sim_cmd` and read results from `~/sim_ack`.

Script Editor scripts write their results to `~/script_log`.

### 2. FL data collection (all 8 robots)

In a ROS terminal:

```bash
./run_all_demo.sh          # Nav2 -> IPE -> 8 demos in order, checking readiness at each stage
./run_all_demo.sh stop     # only clean up a previous run
```

A single Ctrl+C shuts down in order: demos → robot stop → IPE → Nav2.

### 3. Anomaly detection / judgment / response scenarios (site C)

Start Nav2 and IPE for domains 7 and 8 (one terminal each), then run the two demos in separate terminals.

```bash
./Nav2/run_nav2.sh 7 8
./ROS-IPE/run_ipe.sh 7 8
ROS_DOMAIN_ID=7 python -u demo_task_executor/<c1 script>
ROS_DOMAIN_ID=8 python -u demo_task_executor/<c2 script>
```

| Scenario | c1 (domain 7) | c2 (domain 8) | Scene preparation |
|---|---|---|---|
| Floor anomaly | `demo_c1_anno_floor.py` | `demo_c2_anno_floor.py` | Bumps created |
| Robot anomaly | `demo_c1_anno_robot.py` | `demo_c2_anno_robot.py` | Bumps removed |
| Capture (segments labeled by seq) | `demo_c1_capture.py` | `demo_c2_capture.py` | Bumps created with `make_c1_bump.py` |

- The floor and robot anomaly scenarios need the judgment AE (separate directory). Verdicts arrive as CINs that the judgment AE writes to the oneM2M command container (command CIN → IPE → `AMR/cmd` → demo).
- In the floor and robot anomaly scenarios, the c2 scripts do not turn off the wheel lobes (`wknob`). Run `make_wheel_lobe.py` once with `MODE = "off"` before starting. The FL and capture demos turn them off themselves at start.

### Running components individually

```bash
./Nav2/run_nav2.sh [domain...]       # all if omitted; creates launch/ on first run
./ROS-IPE/run_ipe.sh [domain...]     # all robots in fleet.yaml if omitted
./Nav2/stop_all.sh [domain...]       # cancel Nav2 goals + stop (stop the demos first)
```

Logs go to `Nav2/logs/` and `ROS-IPE/logs/`. The IPE also keeps a copy of the telemetry it uploads in `~/telemetry_dump/<AE>_<time>.jsonl`. To disable this, add `dump_dir: ""` to `ROS-IPE/ipe_common.yaml`.

### CSE outage

When the CSE cannot be reached, the IPE keeps uploads in a buffer (up to `buffer_s`, default 30 min). After reconnecting, it sends them in order. Once the backlog is sent, it posts one record to `<AE>/log`:

```json
{"robot": "siteC_AMR_1", "site": "mep-c", "disconnect_duration_ms": 3240, "buffered_count": 3, "data_loss": 0}
```

With `binding: ws`, a dropped network is detected by ping/pong within `ws.dead_s` (5 s).

With `iot.url` set, the IPE asks the MEP for the current CSE every `iot.check_s` (5 s) while disconnected:

| MEP answer | Meaning | Action |
|---|---|---|
| none | network (AP) down | keep buffering and waiting |
| same CSE | CSE restarting | keep retrying the same CSE |
| other CSE | CSE moved | switch to it, re-create the AE, containers, ACPs, pollingChannel and subscription, then send the backlog |

The CSE is taken from `customServicesTransportInfo` entries whose `id` is a CSE-ID (`/…`): `endpoint.uris[0]` + `/` + `name`, and `/ws` for the WebSocket. Outages are detected from failed uploads, so a robot that is standing still (no telemetry) is not checked until it moves. Subscriptions that other AEs made on the old CSE must be re-created by those AEs.

## Notes

- The driving settings in `Nav2/nav2_drive_param.yaml` (`max_velocity`, `allow_reversing`, ...) must stay the same as during FL data collection.
- `~/sim_cmd`, `~/sim_ack`, `~/sim_cmd.lock` and `~/script_log` are temporary files in the home folder, outside the repo. Isaac and the demos must run under the same user account.
- Both bump scripts build under the same `/World/floor_bumps`. Running one removes the other's bumps.
