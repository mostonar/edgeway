#!/usr/bin/env python3
"""EdgeWay local dashboard. No package installation required; bind to loopback by default."""
from __future__ import annotations

import argparse
import json
import logging
import signal
import secrets
import socket
import sqlite3
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

from core import ROOT, VERSION, CSEError, Store, cse_request, dumps, load_config, strict_json, utc_now
from bindings import BindingService, camera_config, project, validate_fields
from operations import Operations, Conflict
from demo_commands import DemoCommands
import factory_c_address
from floor_judgement import FloorJudgement, factories, site_url as floor_site_url
from analysis_verdict import AnalysisVerdict
from cse_handover import Handover
from dashboard_clean import DashboardStore

LOG = logging.getLogger("edgeway")
MAX_BODY = 1024 * 1024
# The GET collector commits from another process and cannot notify this Condition.
# Bound idle SSE delivery delay without changing command/notification handling.
STREAM_IDLE_POLL_SECONDS = 0.25
STATIC = {"/": ("index.html", "text/html; charset=utf-8"),
          "/app.js": ("app.js", "text/javascript; charset=utf-8"),
          "/styles.css": ("styles.css", "text/css; charset=utf-8"),
          "/fonts.css": ("fonts.css", "text/css; charset=utf-8"),
          "/favicon.svg": ("favicon.svg", "image/svg+xml")}
STATIC.update({"/fonts/" + p.name: ("fonts/" + p.name, "font/woff2") for p in (ROOT / "static/fonts").glob("*.woff2")})
STATIC.update({"/settings": ("settings.html", "text/html; charset=utf-8"),
               "/settings.js": ("settings.js", "text/javascript; charset=utf-8"),
               "/settings.css": ("settings.css", "text/css; charset=utf-8"),
               "/camera-preview": ("camera-preview.html", "text/html; charset=utf-8"),
               "/camera-preview.js": ("camera-preview.js", "text/javascript; charset=utf-8"),
               "/data-panels.js": ("data-panels.js", "text/javascript; charset=utf-8")})
STATIC.update({"/floorplan.js": ("floorplan.js", "text/javascript; charset=utf-8"),
               "/floorplan.css": ("floorplan.css", "text/css; charset=utf-8"),
               "/recordings.json": ("recordings.json", "application/json; charset=utf-8")})
STATIC["/admin-actions.js"] = ("admin-actions.js", "text/javascript; charset=utf-8")
STATIC["/event-log-summary.js"] = ("event-log-summary.js", "text/javascript; charset=utf-8")
STATIC["/robot-state.js"] = ("robot-state.js", "text/javascript; charset=utf-8")
STATIC["/floor-judgement.js"] = ("floor-judgement.js", "text/javascript; charset=utf-8")
STATIC["/floor-judgement.css"] = ("floor-judgement.css", "text/css; charset=utf-8")
STATIC["/factory-c-address.js"] = ("factory-c-address.js", "text/javascript; charset=utf-8")
STATIC["/control-settings.js"] = ("control-settings.js", "text/javascript; charset=utf-8")
STATIC.update({"/tests/network-status": ("network-status-test.html", "text/html; charset=utf-8"),
               "/tests/network-status.css": ("network-status-test.css", "text/css; charset=utf-8"),
               "/tests/network-status.js": ("network-status-test.js", "text/javascript; charset=utf-8")})
STATIC.update({f"/floorplan-images/{name}.png": (f"floorplan-images/{name}.png", "image/png")
               for name in ("robot", "collision", "pickup", "carrying", "dropoff", "blocked")})
STATIC.update({f"/floorplan-images/status-{name}.svg":
               (f"floorplan-images/status-{name}.svg", "image/svg+xml")
               for name in ("blocked", "pickup", "carrying", "dropoff", "fault", "collision")})
STATIC.update({f"/factory-maps/{site}.{ext}": (f"factory-maps/{site}.{ext}", mime)
               for site in ("siteA", "siteB", "siteC")
               for ext, mime in (("png", "image/png"), ("yaml", "text/plain; charset=utf-8"))})


class Dashboard(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, cfg):
        self.cfg = cfg
        self.store = DashboardStore(cfg["database"])
        self.bindings = BindingService(cfg, self.store)
        self.operations = Operations(cfg, self.store, self.bindings)
        self.demo_commands = DemoCommands(cfg, self.store)
        # Judgement/detection per factory: C always, A and B when collector_cse_urls has their CSE.
        self.floor_judgements = {f: FloorJudgement(cfg, self.store, f) for f in factories(cfg)}
        self.analysis_verdicts = {f: AnalysisVerdict(cfg, self.store, j) for f, j in self.floor_judgements.items()}
        self.floor_judgement, self.analysis_verdict = self.floor_judgements['C'], self.analysis_verdicts['C']
        # CSE handover (MEC 033): the configured address of each site, before any saved move.
        self.handover = Handover(cfg, self.store, {
            'C': lambda: (lambda c: c['url'] if c['preset'] != 'local' else None)(
                factory_c_address.selection(cfg, self.store, handover=False)),
            **{f: (lambda f=f: floor_site_url(cfg, f)) for f in ('A', 'B') if f in self.floor_judgements}})
        self.csrf = secrets.token_urlsafe(32)
        self.changed = threading.Condition()
        self.stopping = threading.Event()
        self.stream_slots = threading.BoundedSemaphore(8)
        self.cse_status = {"state": "checking", "checked_at": None, "message": "ACME Checking"}
        super().__init__((cfg["host"], cfg["port"]), Handler)
        threading.Thread(target=self.monitor_cse, daemon=True).start()
        self.operations.start()
        for service in (*self.floor_judgements.values(), *self.analysis_verdicts.values()):
            service.start()
        self.handover.start()

    def monitor_cse(self):
        while not self.stopping.is_set():
            try:
                response = cse_request(self.cfg, "GET")
                base = response.get("m2m:cb", {})
                if base.get("csi") != self.cfg["cse_id"]:
                    raise CSEError("Configured CSE-ID differs from the response.")
                status = {"state": "reachable", "message": "ACME Query successful"}
            except (CSEError, ValueError, AttributeError) as exc:
                status = {"state": "unreachable", "message": str(exc)}
            self.cse_status = {**status, "checked_at": utc_now()}
            self.stopping.wait(10 if status["state"] == "reachable" else 2)

    def stop(self):
        self.stopping.set()
        for service in (*self.floor_judgements.values(), *self.analysis_verdicts.values(), self.handover):
            service.stopping.set()
        self.operations.stopping.set()
        with self.changed:
            self.changed.notify_all()
        self.shutdown()


class Handler(BaseHTTPRequestHandler):
    server_version = "EdgeWay/" + VERSION
    sys_version = ""
    protocol_version = "HTTP/1.1"

    def setup(self):
        super().setup()
        self.connection.settimeout(15)

    def log_message(self, fmt, *args):
        # Payloads and headers are deliberately not written to the console.
        LOG.debug("HTTP %s", fmt % args)

    def common_headers(self):
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Referrer-Policy", "no-referrer")
        camera = self.server.store.setting("camera", {"url": ""})
        parsed = urlsplit(camera.get("url", ""))
        camera_origin = f"{parsed.scheme}://{parsed.netloc}" if parsed.netloc else ""
        self.send_header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-src 'self' " + camera_origin + "; frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
        self.send_header("Cache-Control", "no-store")

    def reply(self, status, value, rsc=None):
        data = dumps(value).encode("utf-8")
        self.send_response(status)
        self.common_headers()
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        if rsc:
            self.send_header("X-M2M-RSC", str(rsc))
            self.send_header("X-M2M-RI", self.headers.get("X-M2M-RI", "")[:200])
            self.send_header("X-M2M-RVI", self.server.cfg["release"])
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        try:
            self.get()
        except (BrokenPipeError, ConnectionResetError, TimeoutError):
            pass
        except (ValueError, OverflowError):
            self.reply(400, {"error": "Invalid query parameters."})
        except sqlite3.Error:
            LOG.exception("Database read failed")
            self.reply(503, {"error": "Could not read storage. Check the server console."})

    def get(self):
        url = urlsplit(self.path)
        query = parse_qs(url.query)
        if url.path in STATIC:
            filename, mime = STATIC[url.path]
            try:
                data = (ROOT / "static" / filename).read_bytes()
            except FileNotFoundError:
                self.reply(404, {"error": "Map files are not installed yet."})
                return
            self.send_response(200)
            self.common_headers()
            self.send_header("Content-Type", mime)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        elif url.path == "/health":
            self.reply(200, {"status": "ready", "version": VERSION, "cse_url": self.server.cfg["cse_url"]})
        elif url.path == "/api/status":
            cfg, store = self.server.cfg, self.server.store
            profiles = self.server.bindings.rows()
            robots = {} if profiles else {r["id"]: {"id": r["id"], "name": r["name"], "prepared": False} for r in cfg["robots"]}
            for p in profiles:
                prepared = robots.get(p["robot_id"], {}).get("prepared", False) or p["state"] == "active"
                robots[p["robot_id"]] = {"id": p["robot_id"], "name": p["name"], "prepared": prepared}
            for robot in store.recorded_robots():
                robots.setdefault(robot["id"], robot)
            self.reply(200, {"version": VERSION, "cse": self.server.cse_status, "handover": self.server.handover.snapshot(),
                            "robots": list(robots.values()),
                            "subscriptions": {"registered": sum(p["state"] == "active" for p in profiles), "total": len(profiles)},
                            **store.statistics()})
        elif url.path == "/api/records/reset":
            self.reply(200, {"csrf": self.server.csrf, **self.server.store.records_reset_preview()})
        elif url.path == "/api/floorplan/test-records":
            self.reply(200, {"csrf": self.server.csrf,
                            **self.server.store.test_records_preview(self.server.cfg["cse_url"])})
        elif url.path == "/api/analysis-verdict":
            factory = query.get("factory", ["C"])[0]
            service = self.server.analysis_verdicts.get(factory)
            self.reply(200, service.snapshot() if service else {"enabled": False, "factory": factory})
        elif url.path == "/api/floor-judgement":
            factory = query.get("factory", ["C"])[0]
            service = self.server.floor_judgements.get(factory)
            self.reply(200, {"csrf": self.server.csrf, **service.snapshot()} if service
                       else {"csrf": self.server.csrf, "enabled": False, "factory": factory})
        elif url.path == "/api/handover":
            self.reply(200, self.server.handover.snapshot())
        elif url.path == "/api/factory-c/address":
            self.reply(200, {"csrf": self.server.csrf, **factory_c_address.snapshot(self.server.cfg, self.server.store)})
        elif url.path == "/api/settings":
            cfg = self.server.cfg
            self.reply(200, {"csrf": self.server.csrf, "connection": {k: cfg[k] for k in ("cse_url", "cse_id", "notification_url")},
                             "bindings": self.server.bindings.rows(), "camera": self.server.store.setting("camera", {"name": "Site Camera", "url": "", "mode": "web_viewer"})})
        elif url.path == "/api/bindings/values":
            self.reply(200, {"items": self.server.bindings.values()})
        elif url.path.startswith("/api/bindings/") and url.path.endswith("/sample"):
            self.reply(200, self.server.bindings.sample(url.path.split("/")[3]))
        elif url.path == "/api/control/settings":
            self.reply(200, {"csrf": self.server.csrf, **self.server.operations.settings()})
        elif url.path == "/api/camera":
            self.reply(200, self.server.store.setting("camera", {"name": "Site Camera", "url": "", "mode": "web_viewer"}))
        elif url.path.startswith('/api/events/') and url.path.endswith('/demo-command'):
            event_id = int(url.path.split('/')[3])
            self.reply(200, {"csrf": self.server.csrf, **self.server.demo_commands.preview(event_id)})
        elif url.path == "/api/operations":
            ids = query.get("ids", [""])[0].split(",")
            if len(ids) > 200 or any(not i.isdigit() for i in ids if i):
                raise ValueError("Invalid event query parameters.")
            states = [self.server.operations.snapshot(int(i)) for i in ids if i]
            self.reply(200, {"items": [{k:s[k] for k in ('event_id','status','version','type')} for s in states]})
        elif url.path.startswith('/api/events/') and url.path.endswith('/operations'):
            event_id = int(url.path.split('/')[3])
            self.reply(200, {"csrf": self.server.csrf, **self.server.operations.snapshot(event_id)})
        elif url.path == "/api/events/stream":
            self.stream(query)
        elif url.path == "/api/events":
            before = int(query["before"][0]) if "before" in query else None
            limit = int(query.get("limit", ["50"])[0])
            if not 1 <= limit <= 100 or (before is not None and before < 1):
                raise ValueError("Invalid cursor")
            self.reply(200, self.server.store.events(query.get("robot", [""])[0], before, limit))
        elif url.path.startswith("/api/events/") and url.path.removeprefix("/api/events/").isdigit():
            event = self.server.store.detail(int(url.path.rsplit("/", 1)[1]))
            if event:
                profile = next((p for p in self.server.store.profiles(event["source"])
                                if p.get("container_ri") == event["parent_ri"]), None)
                event["display_values"] = project(event["content"], profile["fields"]) if profile else []
            self.reply(200 if event else 404, event or {"error": "Record not found."})
        else:
            self.reply(404, {"error": "URL not found."})

    def stream(self, query):
        cursor = int(self.headers.get("Last-Event-ID") or query.get("after", ["0"])[0])
        revision = int(query["records_revision"][0]) if "records_revision" in query else None
        if revision is not None and revision < 0:
            raise ValueError("Invalid records revision")
        if cursor < 0:
            raise ValueError("Invalid cursor")
        if not self.server.stream_slots.acquire(blocking=False):
            self.reply(503, {"error": "At most 8 live views can be open."})
            return
        try:
            self.send_response(200)
            self.common_headers()
            self.send_header("Content-Type", "text/event-stream; charset=utf-8")
            self.send_header("Connection", "close")
            self.end_headers()
            self.close_connection = True
            self.wfile.write(b"retry: 2000\n\n")
            self.wfile.flush()
            while not self.server.stopping.is_set():
                current = self.server.store.setting("test_records_revision", 0)
                if revision is not None and current != revision:
                    self.wfile.write(f"event: records-state\ndata: {dumps({'revision': current})}\n\n".encode())
                    revision = current
                rows = self.server.store.after(cursor)
                if rows:
                    for row in rows:
                        frame = f"id: {row['id']}\nevent: log\ndata: {dumps(row)}\n\n"
                        self.wfile.write(frame.encode("utf-8"))
                        cursor = row["id"]
                else:
                    self.wfile.write(b": keepalive\n\n")
                self.wfile.flush()
                if not rows:
                    with self.server.changed:
                        self.server.changed.wait(timeout=STREAM_IDLE_POLL_SECONDS)
        except (BrokenPipeError, ConnectionResetError, TimeoutError, OSError):
            pass
        finally:
            self.server.stream_slots.release()

    def notification_service(self, analysis):
        """The factory instance that owns the callback token (unknown tokens go to C, which rejects them)."""
        path = urlsplit(self.path).path
        token = path.removeprefix('/onem2m/analysis-verdict/' if analysis else '/onem2m/floor-judgement/')
        services = self.server.analysis_verdicts if analysis else self.server.floor_judgements
        return next((s for s in services.values() if s.owns(token)), services['C'])

    def floor_notification(self):
        self.close_connection = True
        analysis = urlsplit(self.path).path.startswith('/onem2m/analysis-verdict/')
        service = self.notification_service(analysis)
        trace = {'received_at': utc_now(), 'peer': self.client_address[0],
                 'origin': self.headers.get('X-M2M-Origin','')[:200],
                 'content_type': self.headers.get_content_type()[:120], 'verification': False}
        status, rsc = 503, 5000
        try:
            if self.headers.get('Origin'):
                raise PermissionError('브라우저에서 구독 알림을 보낼 수 없습니다.')
            if self.headers.get('Transfer-Encoding') or len(self.headers.get_all('Content-Length', [])) != 1:
                raise ValueError('Content-Length가 필요합니다.')
            size = int(self.headers['Content-Length'])
            if not 0 < size <= MAX_BODY:
                raise ValueError('알림 크기는 1 MiB 이하여야 합니다.')
            if self.headers.get_content_type() not in ('application/json', 'application/vnd.onem2m-ntfy+json', 'application/vnd.onem2m-res+json'):
                raise ValueError('JSON 알림이 필요합니다.')
            if not self.headers.get('X-M2M-RI') or len(self.headers['X-M2M-RI']) > 200:
                raise ValueError('X-M2M-RI가 필요합니다.')
            raw = self.rfile.read(size)
            if len(raw) != size:
                raise ValueError('불완전한 알림입니다.')
            path = urlsplit(self.path).path
            analysis = path.startswith('/onem2m/analysis-verdict/')
            prefix = '/onem2m/analysis-verdict/' if analysis else '/onem2m/floor-judgement/'
            service = self.notification_service(analysis)
            payload = strict_json(raw)
            sgn = payload.get('m2m:sgn') if isinstance(payload,dict) else None
            trace['verification'] = isinstance(sgn,dict) and sgn.get('vrq') is True
            service.notification(path.removeprefix(prefix), self.headers.get('X-M2M-Origin', ''), payload)
            status, rsc = 200, 2000
            self.reply(status, {}, rsc)
        except PermissionError as exc:
            status,rsc=403,4103; trace['error']=str(exc)[:240]
            self.reply(status, {'error':str(exc)}, rsc)
        except (ValueError, TypeError, UnicodeError, RecursionError, AttributeError) as exc:
            status,rsc=400,4000; trace['error']=str(exc)[:240]
            self.reply(status, {'error':str(exc)}, rsc)
        except (OSError, sqlite3.Error):
            status,rsc=503,5000; trace['error']='Notification connection/storage failure.'
            self.reply(status, {'error':'판정 알림을 저장하지 못했습니다.'}, rsc)
        finally:
            trace.update(http=status,rsc=rsc)
            service.last_callback=trace
            LOG.info('AE_CALLBACK service=%s peer=%r verification=%s origin=%r http=%s rsc=%s error=%r',
                     'analysis' if analysis else 'judge',trace['peer'],trace['verification'],
                     trace['origin'],status,rsc,trace.get('error',''))

    def do_POST(self):
        if urlsplit(self.path).path.startswith(('/onem2m/floor-judgement/', '/onem2m/analysis-verdict/')):
            self.floor_notification()
            return
        if urlsplit(self.path).path.startswith("/api/"):
            self.settings_write()
            return
        # One request per connection: reject ambiguous framing without leaving unread bytes.
        self.close_connection = True
        try:
            if urlsplit(self.path).path != "/onem2m/notify":
                self.reply(404, {"error": "Not a notification endpoint."}, 4004)
                return
            if self.headers.get("Transfer-Encoding") or len(self.headers.get_all("Content-Length", [])) != 1:
                raise ValueError("Exactly one Content-Length header is required.")
            size = int(self.headers["Content-Length"])
            if not 0 < size <= MAX_BODY:
                self.reply(413, {"error": "Notification size: 1 MiB maximum."}, 4000)
                return
            if self.headers.get_content_type() not in ("application/json", "application/vnd.onem2m-ntfy+json", "application/vnd.onem2m-res+json"):
                self.reply(415, {"error": "JSON notifications only."}, 4000)
                return
            if self.headers.get("Origin"):
                self.reply(403, {"error": "Notifications cannot be submitted from a browser."}, 4103)
                return
            cfg = self.server.cfg
            if self.headers.get("X-M2M-Origin", "").rstrip("/") != cfg["cse_id"]:
                self.reply(403, {"error": "Configured CSE does not match the notification source."}, 4103)
                return
            if not self.headers.get("X-M2M-RI") or len(self.headers["X-M2M-RI"]) > 200:
                raise ValueError("X-M2M-RI is required.")
            raw_bytes = self.rfile.read(size)
            if len(raw_bytes) != size:
                raise ValueError("Incomplete notification body.")
            raw = raw_bytes.decode("utf-8")
            payload = strict_json(raw)
            sgn = payload.get("m2m:sgn") if isinstance(payload, dict) else None
            if not isinstance(sgn, dict):
                raise ValueError("m2m:sgn object required.")
            if sgn.get("vrq") is True:
                self.reply(200, {}, 2000)
                return
            if sgn.get("sud") is True:
                LOG.warning("Subscription deletion notification received; run acme_tools.py check.")
                self.reply(200, {}, 2000)
                return
            nev = sgn.get("nev", {})
            if not isinstance(nev, dict) or nev.get("net") != 3:
                raise ValueError("Only CIN creation notifications (net=3) are supported.")
            rep = nev.get("rep", {})
            cin = rep.get("m2m:cin") if isinstance(rep, dict) else None
            if not isinstance(cin, dict) or cin.get("ty") != 4 or "con" not in cin:
                raise ValueError("A CIN with ty=4 and con is required.")
            for key in ("ri", "pi"):
                if not isinstance(cin.get(key), str) or not 1 <= len(cin[key]) <= 256:
                    raise ValueError(f"Valid CIN {key} is required.")
            if "ct" in cin and not isinstance(cin["ct"], str):
                raise ValueError("CIN ct must be a string.")
            sur = sgn.get("sur")
            if not isinstance(sur, str):
                raise ValueError("Subscription reference sur is required.")
            robot = self.server.store.identify(cfg, cin, sur)
            event_id, created = self.server.store.save(cfg, robot, cin, sur, raw)
            if created:
                with self.server.changed:
                    self.server.changed.notify_all()
                LOG.info("CIN saved: robot=%s id=%s cin=%s", robot["id"], event_id, cin["ri"])
            self.reply(200, {}, 2000)
        except (ValueError, UnicodeError, RecursionError, OverflowError) as exc:
            self.reply(400, {"error": str(exc)}, 4000)
        except (sqlite3.Error, OSError) as exc:
            LOG.error("Notification not acknowledged: %s", exc)
            try:
                self.reply(503, {"error": "Could not save record. Check the server console."}, 5000)
            except OSError:
                pass

    def do_PUT(self):
        self.settings_write()

    def do_DELETE(self):
        self.settings_write()

    def settings_write(self):
        self.close_connection = True
        try:
            if not secrets.compare_digest(self.headers.get("X-EdgeWay-CSRF", ""), self.server.csrf):
                self.reply(403, {"error": "Reload the settings page and try again."})
                return
            origin = self.headers.get("Origin")
            if origin and urlsplit(origin).netloc != self.headers.get("Host"):
                self.reply(403, {"error": "Settings can only be changed from the same dashboard."})
                return
            if self.headers.get("Transfer-Encoding") or len(self.headers.get_all("Content-Length", [])) != 1:
                raise ValueError("Valid Content-Length is required.")
            size = int(self.headers["Content-Length"])
            if not 0 < size <= MAX_BODY:
                raise ValueError("Settings requests must not exceed 1 MiB.")
            if self.headers.get_content_type() != "application/json":
                raise ValueError("JSON settings are required.")
            raw = self.rfile.read(size)
            if len(raw) != size:
                raise ValueError("Incomplete request body.")
            data = strict_json(raw)
            if not isinstance(data, dict):
                raise ValueError("Settings must be a JSON object.")
            path, method, service = urlsplit(self.path).path, self.command, self.server.bindings
            parts = path.strip("/").split("/")
            if path == "/api/records/reset" and method == "POST":
                result = self.server.store.reset_records(data)
                with self.server.changed:
                    self.server.changed.notify_all()
            elif path == "/api/floorplan/test-records" and method == "DELETE":
                result = self.server.store.clear_test_records(self.server.cfg["cse_url"], data)
                with self.server.changed:
                    self.server.changed.notify_all()
            elif len(parts) == 4 and parts[:2] == ['api','events'] and parts[3] == 'demo-command' and method == 'POST':
                result = self.server.demo_commands.send(int(parts[2]), data)
            elif len(parts) == 4 and parts[:2] == ['api','events'] and parts[3] == 'actions' and method == 'POST':
                result = self.server.operations.enqueue(int(parts[2]), data)
            elif path == '/api/floor-judgement/clear' and method == 'POST':
                judge = self.server.floor_judgements.get(data.get('factory', 'C'))
                if not judge:
                    raise ValueError('Judgement is not configured for this factory.')
                result = judge.clear(data)
            elif path == "/api/factory-c/address" and method == "PUT":
                result = {"csrf": self.server.csrf, **factory_c_address.save(self.server.cfg, self.server.store, data)}
            elif path == "/api/control/settings" and method == "PUT":
                result = self.server.operations.save_target(data)
            elif path == "/api/control/check" and method == "POST":
                result = self.server.operations.check_target(data)
            elif path == "/api/bindings" and method == "POST":
                result = service.register(data)
            elif path == "/api/connection/check" and method == "POST":
                base = cse_request(self.server.cfg, "GET").get("m2m:cb", {})
                if base.get("csi") != self.server.cfg["cse_id"]:
                    raise ValueError("Retrieved CSE-ID differs from the configuration.")
                result = {"message": "CSE URL and ID verified. Check the last reception time to confirm actual data reception."}
            elif path == "/api/camera" and method == "PUT":
                result = camera_config(data)
                self.server.store.put_setting("camera", result)
            elif len(parts) == 4 and parts[:2] == ["api", "bindings"]:
                key, action = parts[2:]
                if action == "fields" and method == "PUT":
                    result = service.fields(key, data.get("fields"))
                elif action == "preview" and method == "POST":
                    service.get(key)
                    fields = validate_fields(data.get("fields"))
                    content = data["content"] if "content" in data else service.sample(key)["content"]
                    result = {"values": project(content, fields)}
                elif action == "check" and method == "POST":
                    result = service.check(key)
                elif action == "subscribe" and method == "POST":
                    result = service.register(service.get(key))
                elif action == "subscription" and method == "DELETE":
                    result = service.unsubscribe(key)
                else:
                    self.reply(404, {"error": "Settings action not found."})
                    return
            else:
                self.reply(404, {"error": "Settings action not found."})
                return
            self.reply(200, result)
        except Conflict as exc:
            self.reply(409, {"error": str(exc)})
        except CSEError as exc:
            self.reply(502, {"error": str(exc), "http_status": exc.status, "rsc": exc.rsc})
        except (ValueError, TypeError, UnicodeError, RecursionError, OverflowError) as exc:
            self.reply(400, {"error": str(exc)})
        except sqlite3.IntegrityError:
            self.reply(409, {"error": "Robot, data type or CNT already registered."})
        except (sqlite3.Error, OSError):
            LOG.exception("Settings operation failed")
            try:
                self.reply(503, {"error": "Could not save settings. Check the remote subscription, then try again."})
            except OSError:
                pass


def main():
    parser = argparse.ArgumentParser(description="EdgeWay Internal dashboard")
    parser.add_argument("--config", default=str(ROOT / "config.json"))
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    try:
        cfg = load_config(args.config)
        app = Dashboard(cfg)
    except (ValueError, OSError, sqlite3.Error) as exc:
        parser.exit(1, f"Startup failed: {exc}\n Check whether the dashboard port is in use and check config.json. ACME startup is not required to start the dashboard.\n")

    def stop_signal(signum, frame):
        threading.Thread(target=app.stop, daemon=True).start()

    signal.signal(signal.SIGINT, stop_signal)
    signal.signal(signal.SIGTERM, stop_signal)
    print(f"\nEdgeWay {VERSION} · http://127.0.0.1:{cfg['port']}\n Stop: Ctrl+C\n", flush=True)
    try:
        app.serve_forever(poll_interval=0.25)
    finally:
        app.stopping.set()
        app.operations.stopping.set()
        app.server_close()


if __name__ == "__main__":
    main()
