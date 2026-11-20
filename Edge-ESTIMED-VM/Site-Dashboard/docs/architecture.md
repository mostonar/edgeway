# Architecture

| Component | Role |
| --- | --- |
| `server.py`, `core.py` | HTTP endpoints, static files, SQLite storage, live updates to the page |
| `bindings.py`, `operations.py`, `demo_commands.py` | CSE access and operator actions |
| `watch_isaac_cse.py`, `factory_c_address.py` | Collector: latest robot `status`, `telemetry` and `log` |
| `analysis_verdict.py` | Detection results from `ANALYSIS-AE-<n>/cnt-verdict`, shown as floor-suspect markers |
| `floor_judgement.py` | No-go zones from `JUDGE-AE-<n>/cnt-judgement`, clear requests to `JUDGE-AE-<n>/cnt-request` |
| `cse_handover.py` | CSE handover through MEC 033 |
| `subscription_delivery.py` | Creates and checks notification subscriptions |
| `dashboard_clean.py` | Archives old records and their markers |
| `static/` | Dashboard page, maps, network status test screen |

Detection and judgement arrive by subscription. While a subscription is not verified yet, the dashboard polls the latest value instead, which can skip intermediate values.

A detection result creates a floor-suspect marker. A matching judgement zone, or a confirmed fault on the same robot, removes it. A zone is removed from the map once the CSE has stored the clear request.

Outage records come from the robot IPE's `<robot>/log` container (`disconnect_duration_ms`, `buffered_count`, `data_loss`) and are shown only in the Event Log.

The bundled maps and recordings are synthetic. Pretendard is distributed under the [SIL Open Font License](../static/fonts/Pretendard-LICENSE.txt).
