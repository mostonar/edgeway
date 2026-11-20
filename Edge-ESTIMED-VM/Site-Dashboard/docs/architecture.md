# Data flow and validation scope

| Component | Responsibility |
| --- | --- |
| `server.py`, `core.py` | HTTP endpoints, static assets, SQLite persistence, and client event delivery. |
| `bindings.py`, `operations.py`, `demo_commands.py` | CSE bindings and operator actions. |
| `watch_isaac_cse.py`, `factory_c_address.py` | Retrieve latest robot status, telemetry and IPE outage records (`<robot>/log`) and record the source. |
| `analysis_verdict.py` | Read detection results from `ANALYSIS-AE-3/cnt-verdict` and maintain floor-suspect markers. |
| `floor_judgement.py` | Read rectangles from `JUDGE-AE-3/cnt-judgement` and send explicit clear requests to `JUDGE-AE-3/cnt-request`. |
| `subscription_delivery.py` | Create or verify notification subscriptions and retry configured delivery endpoints where supported. |
| `dashboard_clean.py` | Archive eligible received records and dismiss their display markers. |
| `static/` | Dashboard, map assets, and the isolated network status test screen. |

The detection and judgement services can poll for the latest value while a notification subscription is being verified. Polling retrieves the latest value and can miss intermediate values. Verify subscriptions and producer data separately when deploying to a real CSE.

An analysis report can create a floor-suspect marker. A matching judgement rectangle or a confirmed fault for the same robot can dismiss that marker. Judged rectangles follow the judgement and clear-request flow. The dashboard removes a cleared rectangle after the CSE confirms that it stored the clear request.

The Drop-off badge stays visible during unloading. A different robot phase replaces it immediately. The Judgement AE clear dialog uses English labels and feedback.

The network status test screen keeps position freshness separate from disconnect and recovery. Its event row and outage duration are sample data. Live outages come from the robot IPE's `<robot>/log` container (`disconnect_duration_ms`, `buffered_count`, `data_loss`), which the collector stores as Event Log records. The map badge is not connected to them.

The source package is based on the running dashboard. Site addresses were moved to local configuration, and the production map and recordings were replaced with synthetic assets. The package is verified independently of the production deployment. Pretendard is distributed with its accompanying [SIL Open Font License](../static/fonts/Pretendard-LICENSE.txt).
