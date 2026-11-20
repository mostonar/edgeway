# FL-FedAvg — federated learning coordinator

Runs `IN-AE`, the coordinator of the federated learning (FL) demo. It talks only to
the shared IN-CSE (outside the MEC Sandbox) and belongs to no MEP.

Each round it:

1. writes an `fl-command` CIN to `IN-AE/cnt-fl-control`; the three site apps are
   notified through their `<pollingChannel>` on the IN-CSE,
2. collects the site updates from the drop-boxes `IN-AE/cnt-local-updates/cnt-mn1..3`
   (sites A and B send weights, site C sends metrics only),
3. runs FedAvg (equal weight per site, `FL_AGG_WEIGHTING=samples` for sample-count weighting),
4. writes the new global model to `IN-AE/cnt-global-model`,
5. scores it on the fixed validation set `valset_amr.npz` and writes the result to
   `IN-AE/cnt-fl-inference`.

## Setup

```bash
pip install -r Cloud/FL-FedAvg/requirements.txt
cp Cloud/FL-FedAvg/env.local.example .env.local     # repository root, or this folder
# fill in FL_IN_BASE_URL, FL_IN_CSE_NAME (and FL_IN_USER / FL_IN_PASS if the IN-CSE asks for basic auth)
```

The run script reads `.env.local` from this folder first, then from the repository
root. Values already set in the shell win. `.env*` is git-ignored; never commit it.

## Run

```bash
bash Cloud/FL-FedAvg/run_fedavg.sh      # from the repository root
cd Cloud/FL-FedAvg && bash run_fedavg.sh # or from this folder
```

The script works from any directory and only uses files in its own folder, so the
folder also runs on its own when copied elsewhere.

The script:

1. stops a previous IN-AE (port 6000) and empties its global-model files in
   `/tmp/fl_models/global` (`FL_RESET_ON_START=0` skips this),
2. checks the IN-CSE resources (`setup_resources_standard.py --scope in --verify`) and
   creates them if any are missing,
3. starts `in_ae_standard.py`, logging to `Cloud/FL-FedAvg/logs/run_<time>/in_ae.log`.

Start order does not matter. Before round 1 the IN-AE waits for all three site apps
(`Edge-ESTIMED-VM/Local-Model`) to subscribe to `cnt-fl-control`, for up to
`FL_COMMAND_SUBSCRIPTION_WAIT` seconds (900 in this script).

The dashboard (`FL-Dashboard`) reads the global model from the IN-CSE,
so it does not need to run on this host.

Nothing site-level reaches this coordinator beyond what FedAvg needs: weights (sites
A and B) and summary numbers (loss, sample count, current MN-CSE). Window anomaly
scores and verdicts stay on each site's MN-CSE.

## Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `FL_GLOBAL_ROUNDS` | `10` | number of rounds |
| `FL_AGG_WEIGHTING` | `uniform` | `samples` for textbook sample-count FedAvg |
| `FL_COMMAND_SUBSCRIPTION_WAIT` | `900` (script) | seconds to wait for the site apps' subscriptions |
| `FL_VERIFY_ACP` | `1` | startup ACP self-check; it connects to the site MN-CSEs, so the script turns it off when `FL_SANDBOX_HOST` is unset |
| `FL_RESET_ON_START` | `1` | `0` keeps the previous run's global-model files |

## Ports

| Port | Use |
| --- | --- |
| 6000 | IN-AE notification server. Notifications from the IN-CSE arrive through a `<pollingChannel>` and are handed to it locally, so the IN-CSE never has to reach this port. |

## Files

| File | Role |
| --- | --- |
| `in_ae_standard.py` | the coordinator (IN-AE) |
| `aggregator.py` | FedAvg |
| `valset.py`, `valset_amr.npz` | fixed validation set and its scoring |
| `build_valset.py` | builds `valset_amr.npz` from Isaac Sim captures (offline) |
| `setup_resources_standard.py` | creates/checks oneM2M resources; `--scope in` here |
| `config.py`, `amr_schema.py`, `onem2m_utils.py`, `onem2m_pch.py`, `model_cvae.py`, `model_codec.py` | shared modules (the same files also live in the other FL folders) |
| `fl_env.sh`, `run_fedavg.sh` | run script and its helpers |
