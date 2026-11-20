# Site-Dashboard

Site dashboard of EdgeWay. It reads robot status, telemetry, detection results and judgements from each site's oneM2M CSE and shows them on the factory floor plans and in an event log. A Python server (stdlib only) serves the page and keeps received records in SQLite.

## Quick start

Python 3.10 or later. From this directory:

```bash
cp config.example.json config.json
cp demo-command.example.json demo-command.json
python3 server.py --config config.json
```

Open http://127.0.0.1:8090. The page works without a CSE; the event log stays empty until data arrives.

To see a sample outage record without a CSE, open **Floor Plan → Map settings → Network status test** (http://127.0.0.1:8090/tests/network-status) and select **Network recovered**.

## Connect a CSE

Set `cse_url`, `cse_id`, `origin` and `notification_url` in `config.json`. The CSE must be able to reach `notification_url`. See [docs/configuration.md](docs/configuration.md) for all settings, sites A/B and CSE handover.

Run a collector per site in a second terminal, e.g.:

```bash
python3 watch_isaac_cse.py --config config.json --site C --robots siteC_AMR_1 siteC_AMR_2
```

It polls each robot's latest `status`, `telemetry` and `log`. Outage records from the robot IPE (`<robot>/log`) appear in the Event Log as "Network disconnected · <duration>" with the buffered and lost counts.

Robot recovery commands are off by default (`demo-command.json`, `enabled: false`).

## Tests

```bash
python3 -m unittest discover -s tests -v
```

The tests use temporary storage and local fixtures; no CSE or robot is needed.
