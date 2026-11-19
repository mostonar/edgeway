# EdgeWay Dashboard

Developed for the oneM2M MEC International Hackathon 2026.

EdgeWay monitors autonomous mobile robots through oneM2M. It shows robot positions, event history, anomaly markers, and judged floor areas on factory floor plans. The Python server serves the dashboard and stores received records in SQLite. A separate collector retrieves the latest robot status and telemetry.

## Quick start

Use Python 3.10 or later. Run these commands from the repository root:

```bash
cp config.example.json config.json
cp demo-command.example.json demo-command.json
python3 server.py --config config.json
```

Open [http://127.0.0.1:8090](http://127.0.0.1:8090). An ACME CSE is optional for viewing the interface and the network test screen. An empty event log is normal until a CSE or collector supplies data. The server creates the local database on the first run.

For an example you can open immediately, go to **Floor Plan → Map settings → Network status test**, or open [the network status test screen](http://127.0.0.1:8090/tests/network-status). Select **Network recovered** and open the sample row in **Event Log**. Its detail dialog shows the robot name, outage duration, and lost data count. The sample stays in the test screen. Live integration is pending the reporting AE and its payload format.

## Connect a CSE

Set the CSEBase URL, CSE ID, originator, and notification URL in your local `config.json`. If ACME runs in a separate network environment, use a notification address reachable from ACME. See [Configuration and deployment](docs/configuration.md) for the Factory C selector, collectors, and map calibration.

Robot recovery commands are disabled in the example settings. For a C-2 collector on a configured local CSE, run the following in a second terminal:

```bash
python3 watch_isaac_cse.py --config config.json --site C --robots siteC_AMR_2
```

## Testing

From the repository root, run:

```bash
python3 -m unittest discover -s tests -v
```

The tests run against temporary storage and local fixtures. They do not require a lab CSE or send commands to robots. See [Data flow and validation scope](docs/architecture.md) for the distinction between the tested interface and the live integration.
