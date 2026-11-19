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
| `floor_judgement.mn_notification_base` | Optional HTTP(S) base URL used for Factory C detection and judgement callbacks in MN-CSE mode. |

The Factory C collector follows the dashboard selection. Both detection and judgement use this same CSEBase. Set `factory_c.mn_url` before importing a database that already has the `mep-c` selection saved. The collector uses `origin` and `release` from `config.json`; `EDGEWAY_ORIGIN` can override the originator locally.

The optional command configuration defines whether robot recovery commands can be sent, the originator and TLS policy, and the A/B command CSEBase URLs in `sites`. Factory C uses the shared selector. Keep `enabled` set to `false` until the intended command route has been verified. A successful CSE response confirms that the command was stored. Verify robot execution separately.

## Maps and records

The bundled maps and recordings are synthetic. Configure a real site's map through **Map settings** using the original PNG and the corresponding YAML. Confirm the resolution, origin, and coordinate frame against the robot position data before using the map for operation.

Keep the production configuration, recordings, application database, logs, and backups in the deployment environment. For a new deployment, start with a clean database or restore an authorized backup separately from the source checkout.
