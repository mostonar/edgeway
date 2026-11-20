"""Operator commands: durable audit, explicit CSE routing and correlated results.
The v1 wire format is a proposed integration contract, not an installed ROS2 IPE.
"""
import json
import copy
import re
import threading
import uuid
from datetime import datetime, timezone

from core import CSEError, cse_request, utc_now
from bindings import BindingService

CATALOG = {
    'traffic_restriction': ('Keep restriction', {'release_restriction': 'Release restriction'}),
    'robot_fault': ('Inspection required', {'inspection_complete': 'Inspection complete', 'repair_complete': 'Repair complete'}),
    'judgment_deferred': ('Awaiting review', {'approve_passage': 'Allow passage', 'keep_restriction': 'Keep restriction', 'request_inspection': 'Request inspection'}),
}
DONE = {'release_restriction': 'Restriction released', 'inspection_complete': 'Inspection complete', 'repair_complete': 'Repair complete',
        'approve_passage': 'Passage approved', 'keep_restriction': 'Restriction retained', 'request_inspection': 'Inspection requested'}
LABELS = {'queued': 'Queued', 'sending': 'Sending', 'sent': 'Awaiting robot response', 'failed': 'Failed', 'unknown': 'Execution unconfirmed'}
PATH = re.compile(r'[A-Za-z0-9_-]{1,80}(?:/[A-Za-z0-9_-]{1,80}){1,8}')


def event_type(event):
    content = event['content']
    if isinstance(content, str):
        try: content = json.loads(content)
        except (ValueError, RecursionError): pass
    if isinstance(content, dict):
        description = content.get('event', {})
        kind = description.get('type') if isinstance(description, dict) else None
        if isinstance(kind, str) and kind in CATALOG:
            return kind
    # Legacy messages are matched exactly; sensor values never infer a control action.
    title = event['display']['title']
    return {'통행 제한 감지': 'traffic_restriction', '구동부 이상 감지': 'robot_fault', '판단 유보': 'judgment_deferred',
            'Traffic restriction detected': 'traffic_restriction', 'Drive fault detected': 'robot_fault', 'Assessment deferred': 'judgment_deferred'}.get(title)


class Conflict(ValueError):
    pass


class Operations:
    def __init__(self, cfg, store, bindings=None):
        self.cfg, self.store = cfg, store
        self.bindings = bindings if bindings is not None else BindingService(cfg, store)
        self.settings_lock = threading.RLock()
        self.settings_key = 'robot_control:' + cfg['cse_url'] + ':' + cfg['cse_id']
        self.control = store.setting(self.settings_key, cfg.get('robot_control', {}))
        if not isinstance(self.control, dict):
            raise ValueError('robot_control must be a settings object.')
        self.enabled = self.control.get('enabled', False)
        self.targets = self.control.get('targets', {})
        self.timeout = self.control.get('ack_timeout_seconds', 30)
        if type(self.enabled) is not bool or not isinstance(self.targets, dict):
            raise ValueError('Robot control enabled/targets settings are invalid.')
        if type(self.timeout) not in (int, float) or not 1 <= self.timeout <= 300:
            raise ValueError('Robot response timeout must be 1 to 300 seconds.')
        for robot, target in self.targets.items():
            if not isinstance(target, dict) or any(not isinstance(target.get(k), str) or not PATH.fullmatch(target[k]) for k in ('command_path', 'result_path')):
                raise ValueError('Robot control requires command and result CNT paths.')
            if target['command_path'] == target['result_path']:
                raise ValueError('Command and result CNTs must differ.')
        with store.connect() as db:
            db.execute('''CREATE TABLE IF NOT EXISTS operator_commands (
                id TEXT PRIMARY KEY, event_id INTEGER NOT NULL, action TEXT NOT NULL,
                operator TEXT NOT NULL, note TEXT NOT NULL, state TEXT NOT NULL,
                created_at TEXT NOT NULL, updated_at TEXT NOT NULL, remote_ri TEXT NOT NULL DEFAULT '',
                detail TEXT NOT NULL DEFAULT '', command_path TEXT NOT NULL, result_path TEXT NOT NULL,
                request_id TEXT NOT NULL UNIQUE)''')
            db.execute('CREATE INDEX IF NOT EXISTS operator_event ON operator_commands(event_id,created_at)')
        self.stopping = threading.Event()
        self.thread = None
        self.cursor = 0

    @staticmethod
    def validate_target(data):
        robot = data.get('robot_id')
        if not isinstance(robot, str) or not re.fullmatch(r'[A-Za-z0-9_-]{1,80}', robot):
            raise ValueError('Robot ID must contain 1 to 80 letters, digits, underscores or hyphens.')
        target = {}
        for key in ('command_path', 'result_path'):
            value = data.get(key)
            if not isinstance(value, str) or not PATH.fullmatch(value.strip().strip('/')):
                raise ValueError('Enter a CNT path below the CSE, e.g. Robot1/commands. Do not enter a full URL.')
            target[key] = value.strip().strip('/')
        if target['command_path'] == target['result_path']:
            raise ValueError('Command and result CNTs must differ.')
        enabled, timeout = data.get('enabled'), data.get('ack_timeout_seconds')
        if type(enabled) is not bool or type(timeout) not in (int, float) or not 1 <= timeout <= 300:
            raise ValueError('Specify whether control is enabled and a timeout of 1 to 300 seconds.')
        target.update(enabled=enabled, ack_timeout_seconds=timeout)
        return robot, target

    def settings(self):
        with self.settings_lock:
            targets = {robot: {**target, 'enabled': bool(self.enabled and target.get('enabled', True)),
                       'ack_timeout_seconds': target.get('ack_timeout_seconds', self.timeout)}
                       for robot, target in self.targets.items()}
            return {'revision': self.control.get('revision', ''), 'targets': targets,
                    'cse_url': self.cfg['cse_url'], 'source': 'dashboard' if self.store.setting(self.settings_key) is not None else 'config'}

    def check_target(self, data, *, prepare_command=False):
        robot, target = self.validate_target(data)
        def resource(path, wrapper, origin=None):
            response = cse_request(self.cfg, 'GET', path, origin=origin)
            value = response.get(wrapper) if isinstance(response, dict) else None
            if not isinstance(value, dict):
                raise ValueError('CSE returned an invalid resource response: ' + (path or 'CSEBase'))
            return value
        base = resource('', 'm2m:cb')
        if not isinstance(base, dict) or base.get('csi') != self.cfg['cse_id']:
            raise ValueError('Retrieved CSE-ID differs from the configuration.')
        resources = {}
        for key in ('command_path', 'result_path'):
            try:
                cnt = resource(target[key], 'm2m:cnt')
            except CSEError as exc:
                missing_command = key == 'command_path' and exc.status == 404 and exc.rsc == '4004'
                if not missing_command:
                    raise
                if not prepare_command:
                    raise ValueError('Command CNT is missing. Subscribe to the result path, then enable control and save to create it.') from exc
                if not any(p['robot_id'] == robot and p['resource_path'] == target['result_path'] and p['state'] == 'active'
                           for p in self.store.profiles(self.cfg['cse_url'])):
                    raise ValueError('Subscribe to the result path with the same robot ID first. No command path has been created yet.')
                # Only an explicit enabled save provisions a path. Never create on send/check.
                # Reuse subscription provisioning to preserve existing resources and validate ancestors.
                cnt, _ = self.bindings.ensure_container(target[key])
            if not isinstance(cnt, dict) or cnt.get('ty') != 3 or not cnt.get('ri'):
                raise ValueError(target[key] + ': verify this is a CNT path.')
            resources[key] = cnt
        profile = next((p for p in self.store.profiles(self.cfg['cse_url'])
                        if p['robot_id'] == robot and p['resource_path'] == target['result_path'] and p['state'] == 'active'), None)
        if not profile:
            raise ValueError('Subscribe to the result path with the same robot ID first. Use the result subscription button below.')
        sub = resource(target['result_path'] + '/' + profile['subscription_name'], 'm2m:sub', profile.get('origin'))
        if (not isinstance(sub, dict) or sub.get('ty') != 23 or sub.get('ri') != profile.get('subscription_ri')
                or resources['result_path']['ri'] != profile.get('container_ri')
                or sub.get('pi') != profile.get('container_ri') or sub.get('nu') != [self.cfg['notification_url']]
                or sub.get('enc') != {'net': [3]} or sub.get('nct') != 1):
            raise ValueError('The result subscription does not match the current notification URL or CNT. Check its status under Robot Connections.')
        return {'message': 'Command and result CNTs and the result subscription are verified. IPE execution needs separate verification.',
                'robot_id': robot, 'checked_at': utc_now(), 'commands_sent': 0}

    def save_target(self, data):
        robot, target = self.validate_target(data)
        # Serialize route edits with enqueue/send so an in-flight command cannot be rerouted.
        with self.settings_lock:
            if data.get('expected_revision') != self.control.get('revision', ''):
                raise Conflict('Control settings changed in another window. Reload saved settings.')
            old = self.targets.get(robot, {})
            with self.store.connect() as db:
                pending = db.execute("SELECT 1 FROM operator_commands c JOIN events e ON e.id=c.event_id "
                                     "WHERE e.source=? AND e.robot_id=? AND c.state IN ('queued','sending','sent','unknown') LIMIT 1",
                                     (self.cfg['cse_url'], robot)).fetchone()
            same_route = all(target[k] == old.get(k) for k in ('command_path', 'result_path'))
            same_timeout = target['ack_timeout_seconds'] == old.get('ack_timeout_seconds', self.timeout)
            if pending and (not same_route or not same_timeout or target['enabled']):
                raise Conflict('A command is awaiting a result. Wait before changing paths or timeout. You may disable control without changing the current paths.')
            if target['enabled']:
                self.check_target(data, prepare_command=True)
            settings = self.settings()
            targets = copy.deepcopy(settings['targets'])
            targets[robot] = target
            control = {'enabled': any(t['enabled'] for t in targets.values()), 'targets': targets,
                       'ack_timeout_seconds': self.timeout, 'revision': uuid.uuid4().hex}
            self.store.put_setting(self.settings_key, control)
            self.control, self.targets, self.enabled = control, control['targets'], control['enabled']
            return self.settings()

    def route(self, event):
        with self.settings_lock:
            target = self.targets.get(event['robot_id'])
            enabled = self.enabled and target and target.get('enabled', True)
        if event['source'] != self.cfg['cse_url']:
            return None, 'Current CSE differs from the source of this record.'
        if not enabled:
            return None, 'ROS2 IPE control connection must be configured.'
        profiles = self.store.profiles(self.cfg['cse_url'])
        if not any(p['resource_path'] == target['result_path'] and p['robot_id'] == event['robot_id'] and p['state'] == 'active' for p in profiles):
            return None, 'Subscribe to this robot command result CNT first.'
        return target, ''

    def snapshot(self, event_id):
        event = self.store.detail(event_id)
        if not event:
            raise ValueError('Selected event not found.')
        kind = event_type(event)
        with self.store.connect() as db:
            commands = [dict(r) for r in db.execute('SELECT * FROM operator_commands WHERE event_id=? ORDER BY created_at DESC,id DESC LIMIT 30', (event_id,))]
        latest = commands[0] if commands else None
        status = CATALOG[kind][0] if kind else 'Read-only record'
        actions = dict(CATALOG[kind][1]) if kind else {}
        if latest:
            status = DONE[latest['action']] if latest['state'] == 'succeeded' else LABELS[latest['state']]
            if latest['state'] == 'succeeded':
                actions = {'repair_complete': 'Repair complete'} if latest['action'] == 'inspection_complete' else {}
        route, reason = self.route(event)
        blocked = latest and latest['state'] in ('queued', 'sending', 'sent', 'unknown')
        if blocked:
            reason = 'Checking the existing command result. Duplicate sending is blocked.'
        return {'event_id': event_id, 'event_key': f'EV-{event_id:03d}', 'robot_id': event['robot_id'], 'type': kind,
                'status': status, 'version': latest['id'] if latest else '', 'commands': commands,
                'actions': [{'id': action, 'label': label} for action, label in actions.items()],
                'can_send': bool(route and actions and not blocked), 'reason': reason,
                'scope': 'robot_command'}

    def enqueue(self, event_id, data):
        with self.settings_lock:
            return self._enqueue(event_id, data)

    def _enqueue(self, event_id, data):
        action, operator, note, request_id = (data.get(k) for k in ('action','operator','note','request_id'))
        if not isinstance(operator, str) or not 1 <= len(operator.strip()) <= 80:
            raise ValueError('Enter an operator name of 1 to 80 characters.')
        if not isinstance(note, str) or not 1 <= len(note.strip()) <= 1000:
            raise ValueError('Enter a reason of 1 to 1,000 characters.')
        if not isinstance(request_id, str) or not re.fullmatch(r'[A-Za-z0-9-]{16,80}', request_id):
            raise ValueError('Invalid request identifier.')
        with self.store.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            old = db.execute('SELECT * FROM operator_commands WHERE request_id=?', (request_id,)).fetchone()
            if old:
                if (old['event_id'],old['action'],old['operator'],old['note']) != (event_id,action,operator.strip(),note.strip()):
                    raise Conflict('The same request identifier has different content.')
                return dict(old)
            state = self.snapshot(event_id)
            if data.get('expected_version') != state['version']:
                raise Conflict('State changed in another window. Check again.')
            if not state['can_send'] or action not in [a['id'] for a in state['actions']]:
                raise Conflict(state['reason'] or 'This action is not available for this event.')
            target, _ = self.route(self.store.detail(event_id))
            now = utc_now()
            command = dict(id='cmd-'+uuid.uuid4().hex, event_id=event_id, action=action, operator=operator.strip(), note=note.strip(),
                           state='queued',created_at=now,updated_at=now,remote_ri='',detail='',
                           command_path=target['command_path'],result_path=target['result_path'],request_id=request_id)
            db.execute('INSERT INTO operator_commands VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)', tuple(command.values()))
            return command

    def update(self, command_id, state, detail='', remote_ri=''):
        with self.store.connect() as db:
            db.execute("UPDATE operator_commands SET state=?,detail=?,remote_ri=?,updated_at=? WHERE id=? AND state NOT IN ('succeeded','failed')",
                       (state,detail,remote_ri,utc_now(),command_id))

    def start(self):
        # An interrupted HTTP POST may already have executed; never send it again automatically.
        with self.store.connect() as db:
            db.execute("UPDATE operator_commands SET state='unknown',detail='Transmission before server restart was not confirmed.',updated_at=? WHERE state='sending'", (utc_now(),))
        self.thread = threading.Thread(target=self.run, daemon=True)
        self.thread.start()

    def send(self, command):
        with self.settings_lock:
            return self._send(command)

    def _send(self, command):
        event = self.store.detail(command['event_id'])
        route, reason = self.route(event)
        if not route or route['command_path'] != command['command_path'] or route['result_path'] != command['result_path']:
            self.update(command['id'], 'failed', reason or 'Command path changed. The command was not sent.')
            return
        try:
            base = cse_request(self.cfg,'GET').get('m2m:cb',{})
            if base.get('csi') != self.cfg['cse_id']:
                raise ValueError('CSE-ID does not match.')
            cnt = cse_request(self.cfg,'GET',command['command_path']).get('m2m:cnt',{})
            if cnt.get('ty') != 3:
                raise ValueError('The command path is not a CNT.')
        except (CSEError, ValueError, AttributeError) as exc:
            self.update(command['id'],'failed','Pre-send check failed: '+str(exc));return
        self.update(command['id'],'sending')
        content = {'schema':'edgeway.command.v1','command_id':command['id'],'robot_id':event['robot_id'],
                   'action':command['action'],'event':{'id':event['id'],'cin_ri':event['cin_ri'],'source':event['source']},
                   'operator':command['operator'],'reason':command['note'],'issued_at':command['created_at']}
        try:
            response = cse_request(self.cfg,'POST',command['command_path'],{'m2m:cin':{'con':content}},4)
            cin = response.get('m2m:cin',{})
            if cin.get('ty') != 4 or not isinstance(cin.get('ri'),str) or not cin['ri']:
                raise ValueError('CSE returned an invalid command storage response.')
            self.update(command['id'],'sent','Command saved in the CSE. Waiting for the robot completion response.',cin['ri'])
        except (CSEError, ValueError, AttributeError) as exc:
            state = 'failed' if isinstance(exc,CSEError) and 400 <= exc.status < 500 else 'unknown'
            self.update(command['id'],state,str(exc))

    def accept_result(self, event):
        content = event['content']
        if isinstance(content,str):
            try: content=json.loads(content)
            except (ValueError,RecursionError): return
        if not isinstance(content,dict) or content.get('schema') != 'edgeway.command-result.v1': return
        if content.get('status') not in ('succeeded','failed') or not isinstance(content.get('command_id'),str): return
        with self.store.connect() as db:
            row=db.execute('SELECT * FROM operator_commands WHERE id=?',(content['command_id'],)).fetchone()
        if not row or row['state'] not in ('sending','sent','unknown'): return
        original=self.store.detail(row['event_id'])
        if (event['source'] != original['source'] or event['resource_path'] != row['result_path']
                or event['robot_id'] != original['robot_id'] or content.get('robot_id') != original['robot_id']): return
        message=content.get('message','')
        if not isinstance(message,str): return
        self.update(row['id'],content['status'],message[:1000],row['remote_ri'])

    def tick(self):
        # Read stored notifications. A restart replays them without changing their original content.
        for summary in self.store.after(self.cursor):
            self.accept_result(self.store.detail(summary['id']))
            self.cursor=summary['id']
        with self.store.connect() as db:
            queued=db.execute("SELECT * FROM operator_commands WHERE state='queued' ORDER BY created_at LIMIT 1").fetchone()
            waiting=db.execute("SELECT * FROM operator_commands WHERE state='sent'").fetchall()
        if queued and not self.stopping.is_set(): self.send(dict(queued))
        now=datetime.now(timezone.utc)
        for row in waiting:
            event = self.store.detail(row['event_id'])
            timeout = self.targets.get(event['robot_id'], {}).get('ack_timeout_seconds', self.timeout)
            if (now-datetime.fromisoformat(row['updated_at'])).total_seconds() >= timeout:
                self.update(row['id'],'unknown','Robot response timed out. Verify whether the action executed.',row['remote_ri'])

    def run(self):
        import logging
        while not self.stopping.is_set():
            try: self.tick()
            except Exception: logging.getLogger('edgeway').exception('Operator command worker failed')
            self.stopping.wait(.5)
