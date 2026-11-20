"""Explicit demo-only clear_fault publication. CSE storage is not robot completion."""
import json
import re
import ssl
import uuid
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import HTTPSHandler, ProxyHandler, Request, build_opener

from core import NoRedirect, dumps, utc_now
from robot_state import primary_state
import cse_handover
import factory_c_address

BODY = {'m2m:cin': {'con': {'command': 'clear_fault'}}}
LABELS = {'sending': 'Sending', 'sent': 'Request sent', 'failed': 'Request rejected',
          'unknown': 'Transmission unconfirmed'}


def content_object(value):
    # Polling can store a CIN whose con is itself a JSON string. Decode both
    # the database JSON value and that original con before checking its state.
    for _ in range(2):
        if not isinstance(value, str):
            break
        try:
            value = json.loads(value)
        except (ValueError, TypeError):
            return None
    return value if isinstance(value, dict) else None


class DemoCommands:
    def __init__(self, cfg, store):
        self.store = store
        self.cfg = cfg
        config = Path(cfg.get('config_path', Path(__file__).with_name('config.json'))).parent / 'demo-command.json'
        self.config = json.loads(config.read_text(encoding='utf-8')) if config.exists() else {}
        with store.connect() as db:
            db.execute('''CREATE TABLE IF NOT EXISTS demo_command_requests (
                request_id TEXT PRIMARY KEY, event_id INTEGER NOT NULL, robot_id TEXT NOT NULL,
                url TEXT NOT NULL, state TEXT NOT NULL, message TEXT NOT NULL, created_at TEXT NOT NULL)''')
            db.execute("UPDATE demo_command_requests SET state='unknown', message='Server restarted before transmission was confirmed. Do not resend without checking the command CNT.' WHERE state='sending'")

    def target(self, event):
        # The selected log may be telemetry; its state is only a display hint.
        # Authorize from the latest status CIN for the same robot and source.
        with self.store.connect() as db:
            row = db.execute('SELECT content_json FROM events WHERE source=? AND robot_id=? AND resource_path=? ORDER BY id DESC LIMIT 1',
                             (event['source'], event['robot_id'], event['robot_id'] + '/status')).fetchone()
        latest_content = content_object(row['content_json']) if row else None
        latest = primary_state(latest_content.get('state')) if latest_content else None
        if latest != 'inspection_wait':
            return None, 'Latest received status is not inspection_wait. Refresh robot details.'
        robot = event['robot_id']
        match = re.fullmatch(r'site([ABC])_AMR_([1-4])', robot)
        if not match or (match[1] != 'A' and int(match[2]) > 2):
            return None, 'Only A-1..A-4, B-1..B-2 and C-1..C-2 are demo targets.'
        if self.config.get('enabled') is not True:
            return None, 'Demo commands are not enabled in demo-command.json.'
        base = (factory_c_address.selection(self.cfg, self.store)['url'] if match[1] == 'C'
                else self.config.get('sites', {}).get(match[1], '').rstrip('/'))
        if match[1] != 'C' and base:              # follow a saved CSE move (same URL as collector_cse_urls)
            base, _ = cse_handover.resolve(self.cfg, self.store, match[1], base)
        u = urlsplit(base)
        if u.scheme not in ('http', 'https') or not u.hostname or u.username or u.password or u.query or u.fragment:
            return None, 'Configure a valid CSE base URL for this factory.'
        url = base + '/' + robot + '/command'
        if event['source'].rstrip('/') != base:
            return url, 'This record came from a different CSE or replay. Live command sending is disabled.'
        return url, ''

    def preview(self, event_id):
        event = self.store.detail(event_id)
        if not event:
            raise ValueError('Selected record no longer exists. Reload the dashboard.')
        url, reason = self.target(event)
        with self.store.connect() as db:
            rows = [dict(r) for r in db.execute('SELECT * FROM demo_command_requests WHERE event_id=? ORDER BY created_at DESC LIMIT 10', (event_id,))]
            pending = db.execute("SELECT state FROM demo_command_requests WHERE robot_id=? AND state IN ('sending','unknown') LIMIT 1", (event['robot_id'],)).fetchone()
        latest = rows[0] if rows else None
        if pending:
            reason = 'A transmission for this robot is unconfirmed. Check the command CNT before another request.'
        elif latest and latest['state'] == 'sent':
            reason = 'This record already has a sent request. Robot completion is not confirmed.'
        return {'event_id': event_id, 'robot_id': event['robot_id'], 'url': url,
                'body': BODY, 'can_send': bool(url and not reason), 'reason': reason,
                'status': LABELS.get(latest['state'], '') if latest else 'Ready to request',
                'history': rows, 'tls_verified': self.config.get('verify_tls', True) is not False}

    def send(self, event_id, data):
        with factory_c_address.ADDRESS_LOCK:
            return self._send(event_id, data)

    def _send(self, event_id, data):
        request_id = data.get('request_id')
        if not isinstance(request_id, str) or not re.fullmatch(r'[a-zA-Z0-9-]{16,80}', request_id):
            raise ValueError('Invalid request identifier.')
        if data.get('command') != 'clear_fault' or data.get('confirm') is not True:
            raise ValueError('Explicit clear_fault confirmation is required.')
        preview = self.preview(event_id)
        if data.get('robot_id') != preview['robot_id'] or data.get('expected_url') != preview['url']:
            raise ValueError('Target changed. Reload and confirm the robot and command URL again.')
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            old = db.execute('SELECT * FROM demo_command_requests WHERE request_id=?', (request_id,)).fetchone()
            if old:
                if old['event_id'] != event_id or old['url'] != preview['url']:
                    raise ValueError('Request identifier belongs to another target.')
                return dict(old)
            if not preview['can_send']:
                raise ValueError(preview['reason'])
            blocked = db.execute("SELECT 1 FROM demo_command_requests WHERE (robot_id=? AND state IN ('sending','unknown')) OR (event_id=? AND state='sent') LIMIT 1", (preview['robot_id'], event_id)).fetchone()
            if blocked:
                raise ValueError('Another request already exists. Refresh its status.')
            db.execute('INSERT INTO demo_command_requests VALUES (?,?,?,?,?,?,?)',
                       (request_id, event_id, preview['robot_id'], preview['url'], 'sending', '', utc_now()))
        state, message = 'unknown', 'Transmission unconfirmed. Check the command CNT before resending.'
        try:
            context = ssl.create_default_context()
            if self.config.get('verify_tls', True) is False:
                context.check_hostname = False
                context.verify_mode = ssl.CERT_NONE
            opener = build_opener(ProxyHandler({}), NoRedirect(), HTTPSHandler(context=context))
            headers = {'Content-Type': 'application/json;ty=4', 'Accept': 'application/json',
                       'X-M2M-Origin': self.config.get('origin', 'CAdmin'),
                       'X-M2M-RVI': '3', 'X-M2M-RI': 'edgeway-demo-' + uuid.uuid4().hex}
            request = Request(preview['url'], method='POST', data=dumps(BODY).encode('utf-8'), headers=headers)
            with opener.open(request, timeout=8) as response:
                raw = response.read(1048577)
                if len(raw) > 1048576:
                    raise ValueError('CSE response too large.')
                result = json.loads(raw)
                cin = result.get('m2m:cin', {})
                if response.status != 201 or response.headers.get('X-M2M-RSC') != '2001' or cin.get('ty') != 4 or not isinstance(cin.get('ri'), str) or not cin['ri']:
                    raise ValueError('CSE did not confirm CIN creation.')
                state, message = 'sent', 'Request saved in command CNT. Robot fault clearance is not confirmed.'
        except HTTPError as exc:
            state = 'failed' if 300 <= exc.code < 500 else 'unknown'
            message = 'CSE HTTP ' + str(exc.code) + '. ' + ('Request was rejected.' if state == 'failed' else 'Check the command CNT before resending.')
        except Exception as exc:
            message = 'Transmission unconfirmed (' + type(exc).__name__ + '). Check the command CNT before resending.'
        with self.store.connect() as db:
            db.execute('UPDATE demo_command_requests SET state=?,message=? WHERE request_id=?', (state, message, request_id))
            return dict(db.execute('SELECT * FROM demo_command_requests WHERE request_id=?', (request_id,)).fetchone())
