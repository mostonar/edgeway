# Configuration and deployment

Create `config.json` from `config.example.json` in the application directory. Store each deployment's actual values in its local copy. Put `demo-command.json` beside it if you use the optional demonstration command settings.

| Setting in `config.json` | Purpose |
| --- | --- |
| `host`, `port` | The HTTP listener. The example binds to loopback on port 8090. |
| `database` | SQLite path relative to the directory containing the configuration file. |
| `cse_url`, `cse_id`, `origin`, `release` | The dashboard's default oneM2M endpoint and request settings. |
| `notification_url` | The dashboard callback ending in `/onem2m/notify`. The CSE must be able to reach it. |
| `robots` | Robot IDs and their AE and container names for the default dashboard binding. |
| `factory_c.local_url` | Factory C's local test CSEBase. The default is `http://127.0.0.1:8081/SiteB`. |
| `factory_c.mn_url` | Optional site deployment CSEBase. Set it to enable the MN-CSE choice. |
| `factory_c.initial_preset` | Optional first-run choice: `local` or `mep-c`. A saved selection takes precedence. |
| `collector_cse_urls` | Optional per-site URLs for A and B, such as `{"A": "https://factory-a.example.org/cse"}`. The collector uses `--base-url` first, then the site URL, then `cse_url`. |
| `floor_judgement.mn_notification_base` | Optional HTTP(S) base URL used for detection and judgement callbacks in MN-CSE mode (Factory C) and for Factories A and B. |
| `handover` | Optional CSE handover through the MEP's MEC 033 API. See [CSE handover](#cse-handover). |

Detection and judgement run for Factory C and, when `collector_cse_urls` has their CSE, for Factories A and B. Each factory uses `ANALYSIS-AE-<n>/cnt-verdict`, `JUDGE-AE-<n>/cnt-judgement` and `JUDGE-AE-<n>/cnt-request` (A=1, B=2, C=3) on its own CSE, and the floor plan shows the results for the factory on screen.

The Factory C collector follows the dashboard selection. Both detection and judgement use this same CSEBase. Set `factory_c.mn_url` before importing a database that already has the `mep-c` selection saved. The collector uses `origin` and `release` from `config.json`; `EDGEWAY_ORIGIN` can override the originator locally.

The optional command configuration defines whether robot recovery commands can be sent, the originator and TLS policy, and the A/B command CSEBase URLs in `sites`. Factory C uses the shared selector. Keep `enabled` set to `false` until the intended command route has been verified. A successful CSE response confirms that the command was stored. Verify robot execution separately.

## CSE handover

```json
"handover": {
  "iot_urls": {"C": "https://<MEC_HOST>/<SANDBOX>/mep-c/iots/v1/registered_iot_platforms"},
  "interval": 5,
  "verify_tls": false
}
```

The server checks each listed site's CSE every `interval` seconds. Factory C is checked only while the MN-CSE choice is selected. When the CSE does not answer, the server asks that site's MEP for the current CSE. It reads the `customServicesTransportInfo` entry whose `id` is a CSE-ID and builds the address from `endpoint.uris[0]` + `/` + `name`. It uses the scheme and host of the `iot_urls` entry, because the MEP reports an internal host.

| MEP answer | Meaning | Action |
| --- | --- | --- |
| No answer | Network down | Nothing changes. |
| Same CSE | CSE restarting | Nothing changes. |
| Other CSE | CSE moved | The new address is saved. |

Judgement, detection, the GET collector and demo commands switch to the saved address and subscribe again there. If the MEP later reports the configured address, the saved move is removed. The current state is shown in `GET /api/handover` and in `/api/status`.

For Factories A and B, the move applies to the `collector_cse_urls` address. Demo commands follow it only when `demo-command.json` uses the same URL. The collector reads the saved move from the shared database, so the dashboard server must be running.

## Maps and records

The bundled maps and recordings are synthetic. Configure a real site's map through **Map settings** using the original PNG and the corresponding YAML. Confirm the resolution, origin, and coordinate frame against the robot position data before using the map for operation.

Keep the production configuration, recordings, application database, logs, and backups in the deployment environment. For a new deployment, start with a clean database or restore an authorized backup separately from the source checkout.
