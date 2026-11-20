# FL-Dashboard — federated learning dashboard

Web dashboard for the FL demo: round progress, per-site loss, per-site window anomaly
scores against each site's threshold, global-model evaluation per round, site C's
local-only status and MN-CSE handovers. Open `http://<host>:7000`.

It reads two kinds of CSE:

| CSE | What it reads |
| --- | --- |
| shared IN-CSE | round state (`IN-AE/cnt-fl-control`), per-site summaries in the drop-boxes (loss, sample count, current MN-CSE), the global model (`cnt-global-model`), per-round evaluation (`cnt-fl-inference`) |
| each site's MN-CSE | window anomaly scores, labels and threshold (`TRAINING-AE-i/cnt-window-scores`) |

The window scores are a site's detection results, so the site apps keep them on their
own MN-CSE and never send them to the IN-CSE; only weights and summary numbers go there.
Because it reads both the cloud side and the sites, this folder sits at the repository
root rather than under `Cloud/` or `Edge-ESTIMED-VM/`. Run it on a host that can reach the
site MN-CSEs; in this setup that is the Edge VM.
After an MN-CSE handover (scenario 6) it follows the site to the new CSE, using the
address the site app reports in its drop-box upload.

On this sandbox one Edge VM hosts all three sites, so one dashboard shows all three.
In a real deployment each site would run its own.

## Setup

```bash
pip install -r FL-Dashboard/requirements.txt
cp FL-Dashboard/env.local.example .env.local   # repository root, or this folder
# fill in FL_SANDBOX_HOST, FL_SANDBOX_NAME, FL_IN_BASE_URL, FL_IN_CSE_NAME (+ FL_IN_USER / FL_IN_PASS)
```

On the Edge VM one `.env.local` at the repository root serves this folder and
`Edge-ESTIMED-VM/Local-Model`.

## Run

```bash
bash FL-Dashboard/run_dashboard.sh       # from the repository root
cd FL-Dashboard && bash run_dashboard.sh  # or from this folder
```

The script works from any directory and only uses files in its own folder, so the
folder also runs on its own when copied elsewhere.

It stops a previous dashboard on the port, then starts `dashboard_server.py`, logging
to `logs/run_<time>/dashboard.log` in this folder. It can start at any time; the page
shows the waiting state until the first round.

## Ports

| Port | Use |
| --- | --- |
| 7000 | dashboard (`FL_DASHBOARD_PORT` changes it). The team's Site-Dashboard uses 8090. |

## Files

| File | Role |
| --- | --- |
| `dashboard_server.py` | backend (Flask, `/stream` server-sent events) |
| `fl_dashboard.html` | the page |
| `config.py`, `amr_schema.py`, `onem2m_utils.py`, `model_cvae.py`, `model_codec.py` | shared modules (the same files also live in the other FL folders) |
| `fl_env.sh`, `run_dashboard.sh` | run script and its helpers |
