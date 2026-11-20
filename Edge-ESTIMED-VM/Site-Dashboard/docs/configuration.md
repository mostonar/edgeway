# Configuration

Copy `config.example.json` to `config.json`. Optional robot commands go in `demo-command.json` next to it.

| Setting | Purpose |
| --- | --- |
| `host`, `port` | HTTP listener (example: `127.0.0.1:8090`) |
| `database` | SQLite path, relative to the config file |
| `cse_url`, `cse_id`, `origin`, `release` | Default oneM2M CSE and request settings |
| `notification_url` | Callback ending in `/onem2m/notify`; the CSE must reach it |
| `robots` | Robot IDs with their AE and container names |
| `factory_c.local_url` | Factory C local test CSE (default `http://127.0.0.1:8081/SiteB`) |
| `factory_c.mn_url` | Factory C MN-CSE on MEC; enables the MN-CSE choice |
| `factory_c.initial_preset` | First-run choice: `local` or `mep-c` |
| `collector_cse_urls` | CSEs of factories A and B, e.g. `{"A": "https://<host>/<cse>"}` |
| `floor_judgement.mn_notification_base` | Callback base for detection and judgement on the MN-CSE and factories A/B |
| `handover` | CSE handover, see below |

## Sites

Each factory uses `ANALYSIS-AE-<n>/cnt-verdict`, `JUDGE-AE-<n>/cnt-judgement` and `JUDGE-AE-<n>/cnt-request` on its own CSE (A=1, B=2, C=3). Factories A and B are enabled when `collector_cse_urls` has their CSE. The floor plan shows the factory on screen.

Factory C follows the CSE chosen in the dashboard (local or MN-CSE); detection, judgement and the collector all use it.

The collector takes its CSE from `--base-url`, then `collector_cse_urls`, then `cse_url`. `EDGEWAY_ORIGIN` overrides the originator.

Robot commands (`demo-command.json`) stay off until `enabled` is `true`. A/B command CSEs are set in `sites`; Factory C uses the dashboard choice.

## CSE handover

```json
"handover": {
  "iot_urls": {"C": "https://<MEC_HOST>/<SANDBOX>/mep-c/iots/v1/registered_iot_platforms"},
  "interval": 5,
  "verify_tls": false
}
```

Every `interval` seconds the server checks each listed site's CSE (Factory C only while the MN-CSE is selected). If it does not answer, the server asks the site's MEP through the MEC 033 IoT API:

| MEP answer | Meaning | Action |
| --- | --- | --- |
| none | network down | wait |
| same CSE | CSE restarting | wait |
| other CSE | CSE moved | switch to the new CSE |

After a move, judgement, detection, the collector and robot commands use the new CSE and subscribe again. When the MEP reports the original CSE again, the dashboard switches back. The state is shown at `GET /api/handover` and in `/api/status`.

For factories A and B the move applies to the `collector_cse_urls` address; robot commands follow only if `demo-command.json` uses the same URL. The collector reads the move from the dashboard database, so the server must be running.

## Maps

The bundled maps are synthetic. For a real site, load its PNG and YAML in **Map settings** and check the resolution, origin and frame against robot positions.
