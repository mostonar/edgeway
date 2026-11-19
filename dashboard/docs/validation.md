# Release validation

The deployment baseline was checked on 2026-11-19. All 13 Python application files matched the checksums reported by the operating server before packaging changes. The server also returned 35 application assets over HTTP, including the main dashboard, map code, and the network recovery preview. The production SQLite database, deployment configuration, logs, maps, and recordings were excluded from the source package.

The shareable tree moves site-specific CSE URLs to local settings, uses the configured collector originator, and includes synthetic maps and a sample recording. The dashboard's runtime file layout is retained.

| Check | Scope |
| --- | --- |
| Dashboard HTTP and static assets | Run against a temporary dashboard and database. Includes health, the test page, sample maps, the bundled font, and a 404 check. |
| Collector and AE data | Local oneM2M fixture supplies status, telemetry, a detection verdict, and a judgement rectangle. The fixture also exercises retrieval while subscription verification fails. |
| CSE selection | Local mode works by default. MN-CSE is available when configured. URLs with queries are rejected. |
| Network recovery UI | Sample selection shows a recovery event. Its detail dialog displays the robot, outage duration, and lost count. The fixture does not represent an actual network outage. |

For the reproducible Python checks, run `python3 -m unittest discover -s tests -v` from the `dashboard/` directory. These checks do not establish connectivity to a deployed CSE, actual robot command execution, or live network recovery delivery.
