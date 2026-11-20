"""Shared configuration, oneM2M HTTP client and durable event storage. Python 3.12+."""
from __future__ import annotations

import json
import hashlib
import os
import re
import sqlite3
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener
from event_display import present

ROOT = Path(__file__).resolve().parent
VERSION = "0.2.1"


def utc_now():
    return datetime.now(timezone.utc).isoformat(timespec="microseconds")


def dumps(value):
    return json.dumps(value, ensure_ascii=False, allow_nan=False, separators=(",", ":"))


def strict_json(value):
    def invalid_constant(text):
        raise ValueError(f"Non-JSON number: {text}")
    return json.loads(value, parse_constant=invalid_constant)


def load_config(path):
    path = Path(path).resolve()
    if not path.is_file():
        raise ValueError(f"Configuration file not found: {path}\nCopy config.example.json to config.json.")
    cfg = strict_json(path.read_text(encoding="utf-8"))
    required = ("host", "port", "database", "cse_url", "cse_id", "origin", "release",
                "notification_url", "subscription_name", "robots")
    if not isinstance(cfg, dict) or any(key not in cfg for key in required):
        raise ValueError("Required settings are missing. Check config.example.json.")
    if cfg["host"] not in ("127.0.0.1", "0.0.0.0"):
        raise ValueError("Set host to 127.0.0.1 or 0.0.0.0 for network access.")
    if type(cfg["port"]) is not int or not 1024 <= cfg["port"] <= 65535:
        raise ValueError("port must be an integer from 1024 to 65535.")
    for key in ("cse_url", "notification_url"):
        u = urlsplit(cfg[key])
        if u.scheme not in ("http", "https") or not u.hostname or u.username or u.password or u.query or u.fragment:
            raise ValueError(f"{key}: An HTTP(S) URL without credentials or query parameters is required.")
    cfg["cse_url"] = cfg["cse_url"].rstrip("/")
    cfg["cse_id"] = "/" + cfg["cse_id"].strip("/")
    if urlsplit(cfg["notification_url"]).path != "/onem2m/notify":
        raise ValueError("notification_url must use the path /onem2m/notify.")
    cse = urlsplit(cfg["cse_url"])
    notification = urlsplit(cfg["notification_url"])
    local_hosts = {"127.0.0.1", "localhost", "::1"}
    if cse.hostname in local_hosts and cse.port == cfg["port"]:
        raise ValueError("Local ACME and the dashboard must use different ports.")
    if notification.hostname in local_hosts and notification.port != cfg["port"]:
        raise ValueError("The local notification_url port must match the dashboard port. Change both together.")
    if not isinstance(cfg["robots"], list) or not cfg["robots"]:
        raise ValueError("Specify at least one robot in robots.")
    ids, paths = set(), set()
    for robot in cfg["robots"]:
        for key in ("id", "ae", "container"):
            if not isinstance(robot.get(key), str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", robot[key]):
                raise ValueError(f"Robot {key} must contain 1 to 80 letters, digits, underscores or hyphens.")
        if not isinstance(robot.get("name"), str) or not 1 <= len(robot["name"]) <= 80:
            raise ValueError("Robot name must contain 1 to 80 characters.")
        pair = (robot["ae"], robot["container"])
        if robot["id"] in ids or pair in paths:
            raise ValueError("Duplicate robot ID or AE/CNT path.")
        ids.add(robot["id"])
        paths.add(pair)
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", cfg["subscription_name"]):
        raise ValueError("subscription_name contains unsupported characters.")
    cfg["origin"] = os.environ.get("EDGEWAY_ORIGIN", cfg["origin"])
    cfg["database"] = str((path.parent / cfg["database"]).resolve())
    cfg["config_path"] = str(path)
    return cfg


class CSEError(Exception):
    def __init__(self, message, status=0, rsc=None):
        super().__init__(message)
        self.status, self.rsc = status, rsc


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def cse_request(cfg, method, suffix="", body=None, ty=None, origin=None):
    url = cfg["cse_url"] + ("/" + quote(suffix.strip("/"), safe="/") if suffix else "")
    headers = {"Accept": "application/json", "X-M2M-Origin": origin or cfg["origin"],
               "X-M2M-RI": "edgeway-" + uuid.uuid4().hex, "X-M2M-RVI": cfg["release"]}
    data = None
    if body is not None:
        data = dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json" + (f";ty={ty}" if ty else "")
    req = Request(url, data=data, headers=headers, method=method)
    opener = build_opener(ProxyHandler({}), NoRedirect())
    try:
        with opener.open(req, timeout=4) as response:
            status, rsc = response.status, response.headers.get("X-M2M-RSC")
            raw = response.read(2 * 1024 * 1024)
    except HTTPError as exc:
        raise CSEError(f"ACME HTTP {exc.code} / RSC {exc.headers.get('X-M2M-RSC', '?')}",
                       exc.code, exc.headers.get("X-M2M-RSC")) from exc
    except (URLError, TimeoutError, OSError) as exc:
        raise CSEError(f"Could not connect to ACME: {exc}") from exc
    if not rsc or not rsc.startswith("2"):
        raise CSEError(f"ACME request failed: HTTP {status} / RSC {rsc}", status, rsc)
    try:
        return strict_json(raw) if raw.strip() else {}
    except (ValueError, UnicodeError) as exc:
        raise CSEError("ACME response is not valid JSON.", status, rsc) from exc


def resource_path(robot):
    return robot.get("_resource_path") or robot["ae"] + "/" + robot["container"]


def subscription_refs(cfg, robot, sub_ri=""):
    structured = urlsplit(cfg["cse_url"]).path.rstrip("/") + "/" + resource_path(robot) + "/" + cfg["subscription_name"]
    refs = {structured, cfg["cse_id"] + structured}
    if sub_ri:
        refs.update({sub_ri, "/" + sub_ri, cfg["cse_id"] + "/" + sub_ri})
    return refs


class Store:
    def __init__(self, path):
        self.path = path
        Path(path).parent.mkdir(parents=True, exist_ok=True)
        with self.connect() as db:
            db.execute("PRAGMA journal_mode=WAL")
            db.executescript("""
                CREATE TABLE IF NOT EXISTS bindings (
                    source TEXT NOT NULL, robot_id TEXT NOT NULL, container_ri TEXT NOT NULL,
                    subscription_ri TEXT NOT NULL, ae_origin TEXT NOT NULL,
                    updated_at TEXT NOT NULL, PRIMARY KEY(source,robot_id)
                );
                CREATE TABLE IF NOT EXISTS events (
                    id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL,
                    cse_id TEXT NOT NULL, cin_ri TEXT NOT NULL, parent_ri TEXT NOT NULL,
                    robot_id TEXT NOT NULL, robot_name TEXT NOT NULL, resource_path TEXT NOT NULL,
                    received_at TEXT NOT NULL, cse_created_at TEXT, content_json TEXT NOT NULL,
                    notification_raw TEXT NOT NULL, subscription_ref TEXT NOT NULL,
                    UNIQUE(source,cin_ri)
                );
                CREATE TABLE IF NOT EXISTS cleared_test_events (
                    source TEXT NOT NULL, cin_ri TEXT NOT NULL, event_id INTEGER NOT NULL,
                    cleared_at TEXT NOT NULL, row_json TEXT NOT NULL,
                    PRIMARY KEY(source,cin_ri)
                );
                CREATE INDEX IF NOT EXISTS events_robot ON events(robot_id,id);
                CREATE INDEX IF NOT EXISTS events_parent ON events(source,parent_ri,id);
                CREATE TABLE IF NOT EXISTS data_bindings (
                    id TEXT PRIMARY KEY, source TEXT NOT NULL, resource_path TEXT NOT NULL,
                    robot_id TEXT NOT NULL, channel TEXT NOT NULL, profile_json TEXT NOT NULL,
                    UNIQUE(source,resource_path), UNIQUE(source,robot_id,channel)
                );
                CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
            """)

    @contextmanager
    def connect(self):
        db = sqlite3.connect(self.path, timeout=5)
        db.row_factory = sqlite3.Row
        try:
            with db:
                yield db
        finally:
            db.close()

    def binding(self, source, robot_id):
        with self.connect() as db:
            row = db.execute("SELECT * FROM bindings WHERE source=? AND robot_id=?", (source, robot_id)).fetchone()
            return dict(row) if row else None

    def bind(self, cfg, robot, container_ri, sub_ri, ae_origin):
        with self.connect() as db:
            db.execute("""INSERT INTO bindings VALUES (?,?,?,?,?,?)
                ON CONFLICT(source,robot_id) DO UPDATE SET container_ri=excluded.container_ri,
                subscription_ri=excluded.subscription_ri,ae_origin=excluded.ae_origin,updated_at=excluded.updated_at""",
                (cfg["cse_url"], robot["id"], container_ri, sub_ri, ae_origin, utc_now()))
        self.import_bindings(cfg)
        for profile in self.profiles(cfg["cse_url"]):
            if profile["resource_path"] == resource_path(robot) and profile["subscription_name"] == cfg["subscription_name"]:
                profile.update(container_ri=container_ri, subscription_ri=sub_ri, origin=ae_origin,
                               state="active", error="")
                self.put_profile(profile)

    def import_bindings(self, cfg):
        """Add legacy CLI mappings once. Inactive UI mappings must stay inactive."""
        with self.connect() as db:
            for robot in cfg["robots"]:
                old = db.execute("SELECT * FROM bindings WHERE source=? AND robot_id=?",
                                 (cfg["cse_url"], robot["id"])).fetchone()
                if not old:
                    continue
                path = resource_path(robot)
                key = "b-" + hashlib.sha256((cfg["cse_url"] + "/" + path).encode()).hexdigest()[:20]
                profile = {"id": key, "source": cfg["cse_url"], "resource_path": path,
                           "robot_id": robot["id"], "name": robot["name"], "channel": "log",
                           "container_ri": old["container_ri"], "subscription_ri": old["subscription_ri"],
                           "subscription_name": cfg["subscription_name"], "origin": old["ae_origin"],
                           "state": "active", "error": "", "fields": [], "updated_at": old["updated_at"]}
                db.execute("INSERT OR IGNORE INTO data_bindings VALUES (?,?,?,?,?,?)",
                           (key, cfg["cse_url"], path, robot["id"], "log", dumps(profile)))

    def profiles(self, source):
        with self.connect() as db:
            return [strict_json(r[0]) for r in db.execute(
                "SELECT profile_json FROM data_bindings WHERE source=? ORDER BY robot_id,channel", (source,))]

    def profile(self, key, source):
        with self.connect() as db:
            row = db.execute("SELECT profile_json FROM data_bindings WHERE id=? AND source=?", (key, source)).fetchone()
            if not row:
                raise ValueError("Registered subscription not found.")
            return strict_json(row[0])

    def put_profile(self, profile):
        profile = {**profile, "updated_at": utc_now()}
        with self.connect() as db:
            db.execute("""INSERT INTO data_bindings VALUES (?,?,?,?,?,?) ON CONFLICT(id)
                          DO UPDATE SET profile_json=excluded.profile_json""",
                       (profile["id"], profile["source"], profile["resource_path"],
                        profile["robot_id"], profile["channel"], dumps(profile)))
        return profile

    def latest(self, profile):
        with self.connect() as db:
            row = db.execute("SELECT * FROM events WHERE source=? AND parent_ri=? ORDER BY id DESC LIMIT 1",
                             (profile["source"], profile.get("container_ri", ""))).fetchone()
            return self.event(row, detail=True) if row else None

    def setting(self, key, default=None):
        with self.connect() as db:
            row = db.execute("SELECT value FROM settings WHERE key=?", (key,)).fetchone()
            return strict_json(row[0]) if row else default

    def put_setting(self, key, value):
        with self.connect() as db:
            db.execute("INSERT INTO settings VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", (key, dumps(value)))

    def identify(self, cfg, cin, sur):
        self.import_bindings(cfg)
        for profile in self.profiles(cfg["cse_url"]):
            robot = {"id": profile["robot_id"], "name": profile["name"], "_resource_path": profile["resource_path"]}
            refs = subscription_refs({**cfg, "subscription_name": profile["subscription_name"]}, robot,
                                     profile.get("subscription_ri", ""))
            if (profile["state"] in ("active", "pending", "unknown") and profile.get("container_ri") == cin["pi"] and sur in refs):
                return robot
        raise ValueError("Does not match a registered robot CNT/subscription. Check acme_tools.py prepare.")

    def save(self, cfg, robot, cin, sur, raw):
        content = dumps(cin["con"])
        with self.connect() as db:
            # Serialize receipt order and commit before acknowledging the CSE.
            db.execute("BEGIN IMMEDIATE")
            old = db.execute("SELECT * FROM events WHERE source=? AND cin_ri=?", (cfg["cse_url"], cin["ri"])).fetchone()
            if old:
                if old["parent_ri"] != cin["pi"] or strict_json(old["content_json"]) != cin["con"]:
                    raise ValueError("Content for this CIN ID differs from the existing record.")
                return old["id"], False
            archived = db.execute("SELECT event_id,row_json FROM cleared_test_events WHERE source=? AND cin_ri=?",
                                  (cfg["cse_url"], cin["ri"])).fetchone()
            if archived:
                original = strict_json(archived["row_json"])
                if original["parent_ri"] != cin["pi"] or strict_json(original["content_json"]) != cin["con"]:
                    raise ValueError("Content for this CIN ID differs from the archived record.")
                return archived["event_id"], False
            cursor = db.execute("""INSERT INTO events
                (source,cse_id,cin_ri,parent_ri,robot_id,robot_name,resource_path,received_at,
                 cse_created_at,content_json,notification_raw,subscription_ref)
                VALUES (?,?,?,?,?,?,?,?,?,?,?,?)""",
                (cfg["cse_url"], cfg["cse_id"], cin["ri"], cin["pi"], robot["id"], robot["name"],
                 resource_path(robot), utc_now(), cin.get("ct"), content, raw, sur))
            return cursor.lastrowid, True

    @staticmethod
    def _test_pose(row):
        """Explicit test telemetry only; never infer test status from a robot or title."""
        try:
            content = strict_json(row["content_json"])
            if isinstance(content, str):
                content = strict_json(content)
            if not isinstance(content, dict) or content.get("test_data") is not True:
                return False
            if not isinstance(content.get("pose"), dict) or "t_sim" not in content:
                return False
            run_id = content.get("run_id")
            if isinstance(run_id, str) and run_id.strip():
                return True
            raw = strict_json(row["notification_raw"])
            labels = raw.get("m2m:sgn", {}).get("nev", {}).get("rep", {}).get("m2m:cin", {}).get("lbl", [])
            return isinstance(labels, list) and any(isinstance(v, str) and v.startswith("run:") and v[4:].strip() for v in labels)
        except (ValueError, TypeError, AttributeError):
            return False

    def _test_records(self, db, source, through):
        protected = set()
        if db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name='operator_commands'").fetchone():
            protected = {r[0] for r in db.execute("SELECT DISTINCT event_id FROM operator_commands")}
        return [r for r in db.execute("SELECT * FROM events WHERE source=? AND id<=? ORDER BY id", (source, through))
                if r["id"] not in protected and self._test_pose(r)]

    @staticmethod
    def _test_preview(rows, through):
        fingerprint = hashlib.sha256(dumps([dict(r) for r in rows]).encode()).hexdigest()
        return {"count": len(rows), "through_id": through, "fingerprint": fingerprint,
                "robots": sorted({r["robot_id"] for r in rows})}

    def test_records_preview(self, source):
        with self.connect() as db:
            db.execute("BEGIN")
            through = db.execute("SELECT COALESCE(MAX(id),0) FROM events").fetchone()[0]
            return self._test_preview(self._test_records(db, source, through), through)

    def clear_test_records(self, source, data):
        through, fingerprint = data.get("through_id"), data.get("fingerprint")
        if type(through) is not int or through < 0 or not isinstance(fingerprint, str):
            raise ValueError("Preview the test records before clearing them.")
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            rows = self._test_records(db, source, through)
            preview = self._test_preview(rows, through)
            if fingerprint != preview["fingerprint"]:
                raise ValueError("The records to clear have changed. Preview them again.")
            stamp = utc_now()
            for row in rows:
                db.execute("INSERT INTO cleared_test_events VALUES (?,?,?,?,?)",
                           (row["source"], row["cin_ri"], row["id"], stamp, dumps(dict(row))))
                db.execute("DELETE FROM events WHERE id=?", (row["id"],))
            if rows:
                value = db.execute("SELECT value FROM settings WHERE key='test_records_revision'").fetchone()
                revision = (strict_json(value[0]) if value else 0) + 1
                db.execute("INSERT INTO settings(key,value) VALUES ('test_records_revision',?) "
                           "ON CONFLICT(key) DO UPDATE SET value=excluded.value", (dumps(revision),))
            return {"deleted": len(rows), "archived": len(rows)}

    def records_reset_preview(self):
        with self.connect() as db:
            db.execute("BEGIN")
            through = db.execute("SELECT COALESCE(MAX(id),0) FROM events").fetchone()[0]
            return self._records_reset_preview(db, through)

    def _records_reset_preview(self, db, through):
        count = db.execute("SELECT COUNT(*) FROM events WHERE id<=?", (through,)).fetchone()[0]
        revision = self._records_revision(db)
        # Stored events are immutable. Revision changes invalidate another clear.
        fingerprint = hashlib.sha256(dumps([through, count, revision]).encode()).hexdigest()
        return {"count": count, "through_id": through, "fingerprint": fingerprint,
                "records_revision": revision, "scope": "all_factories"}

    def reset_records(self, data):
        through, fingerprint = data.get("through_id"), data.get("fingerprint")
        if (data.get("confirm") != "reset_all_records" or type(through) is not int
                or through < 0 or not isinstance(fingerprint, str)):
            raise ValueError("Preview and confirm all dashboard records before resetting.")
        with self.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            preview = self._records_reset_preview(db, through)
            if fingerprint != preview["fingerprint"]:
                raise ValueError("Records changed. Close this dialog and preview again.")
            stamp = utc_now()
            # Reuse the existing recovery archive and CIN replay suppression.
            # Iterate the snapshot without loading all raw telemetry into memory.
            for row in db.execute("SELECT * FROM events WHERE id<=? ORDER BY id", (through,)):
                db.execute("INSERT INTO cleared_test_events VALUES (?,?,?,?,?)",
                           (row["source"], row["cin_ri"], row["id"], stamp, dumps(dict(row))))
            db.execute("DELETE FROM events WHERE id<=?", (through,))
            revision = preview["records_revision"] + 1
            db.execute("INSERT INTO settings(key,value) VALUES ('test_records_revision',?) "
                       "ON CONFLICT(key) DO UPDATE SET value=excluded.value", (dumps(revision),))
            return {"cleared": preview["count"], "archived": preview["count"],
                    "records_revision": revision, "scope": "all_factories"}

    @staticmethod
    def event(row, detail=False):
        data = dict(row)
        data["content"] = strict_json(data.pop("content_json"))
        data["display"] = present(data["content"], data["robot_name"])
        if not detail:
            data.pop("notification_raw", None)
            content = data.pop("content")
            coordinates = content
            if isinstance(content, str):
                try:
                    coordinates = strict_json(content)
                except (ValueError, TypeError):
                    coordinates = None
            # Coordinates travel with the summary so live poses never wait for
            # per-event detail requests. Keep raw sensor arrays in the detail API.
            if isinstance(coordinates, dict) and isinstance(coordinates.get("pose"), dict) and "t_sim" in coordinates:
                pose = {k: coordinates["pose"][k] for k in ("x", "y", "yaw") if k in coordinates["pose"]}
                times = coordinates["t_sim"]
                if (not isinstance(times, list) or len(times) <= 2000) and all(not isinstance(v, list) or len(v) <= 2000 for v in pose.values()):
                    compact = {k: coordinates[k] for k in ("robot", "t_wall", "t_sim", "state", "nav", "run_id", "scenario_id", "event", "event_type") if k in coordinates}
                    compact["pose"] = pose
                    try:
                        raw = strict_json(row["notification_raw"])
                        cin = raw.get("m2m:cin") or raw.get("m2m:sgn", {}).get("nev", {}).get("rep", {}).get("m2m:cin", {})
                        labels = cin.get("lbl", [])
                        if isinstance(labels, list):
                            data["map_labels"] = [v for v in labels if isinstance(v, str) and v.startswith(("run:", "cond:", "load:", "spd:", "scn:"))]
                    except (ValueError, TypeError, AttributeError):
                        pass
                    data["map_content"] = compact
            text = content if isinstance(content, str) else dumps(content)
            data["preview"] = text[:600]
            data["truncated"] = len(text) > 600
        return data

    def events(self, robot="", before=None, limit=50):
        clauses, params = [], []
        if robot:
            clauses.append("robot_id=?")
            params.append(robot)
        if before:
            clauses.append("id<?")
            params.append(before)
        where = " WHERE " + " AND ".join(clauses) if clauses else ""
        with self.connect() as db:
            db.execute("BEGIN")
            latest = db.execute("SELECT COALESCE(MAX(id),0) FROM events").fetchone()[0]
            total = db.execute("SELECT COUNT(*) FROM events" + (" WHERE robot_id=?" if robot else ""), (robot,) if robot else ()).fetchone()[0]
            rows = db.execute("SELECT * FROM events" + where + " ORDER BY id DESC LIMIT ?", (*params, limit + 1)).fetchall()
            return {"events": [self.event(row) for row in rows[:limit]], "latest_id": latest,
                    "total": total, "has_more": len(rows) > limit,
                    "records_revision": self._records_revision(db)}

    @staticmethod
    def _records_revision(db):
        row = db.execute("SELECT value FROM settings WHERE key='test_records_revision'").fetchone()
        return strict_json(row[0]) if row else 0

    def after(self, cursor):
        with self.connect() as db:
            return [self.event(row) for row in db.execute("SELECT * FROM events WHERE id>? ORDER BY id LIMIT 100", (cursor,))]

    def detail(self, event_id):
        with self.connect() as db:
            row = db.execute("SELECT * FROM events WHERE id=?", (event_id,)).fetchone()
            return self.event(row, detail=True) if row else None

    def recorded_robots(self):
        # Include snapshot-imported robots even when they have no subscription binding.
        with self.connect() as db:
            rows = db.execute("SELECT robot_id,robot_name FROM events WHERE id IN "
                              "(SELECT MAX(id) FROM events GROUP BY robot_id) ORDER BY robot_id").fetchall()
            return [{"id": row["robot_id"], "name": row["robot_name"], "prepared": False} for row in rows]

    def statistics(self):
        with self.connect() as db:
            row = db.execute("SELECT COUNT(*) AS total,MAX(received_at) AS last_received_at FROM events").fetchone()
            return dict(row)
