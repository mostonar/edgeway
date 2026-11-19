"""Subscribe to detection verdicts; supplied detection poses are floor-suspect locations."""
import hashlib
import sqlite3
import threading
import time
from urllib.parse import urlsplit
from urllib.error import HTTPError
import uuid

from core import CSEError, dumps, strict_json, utc_now
from robot_state import primary_state
from subscription_delivery import callback_url, ensure_subscription
from floor_judgement import content, number, rectangle_contains, is_rectangle, validate_profile

VERDICTS = 'ANALYSIS-AE-3/cnt-verdict'


def verdict(value, factory):
    value = content(value)
    if not isinstance(value, dict) or value.get('type') != 'amr-verdict':
        raise ValueError('Expected type amr-verdict.')
    robot, result = value.get('robot'), value.get('verdict')
    if not isinstance(robot, str) or not robot.strip():
        raise ValueError('Expected a robot name.')
    if not isinstance(result, str) or not result:
        raise ValueError('Expected a verdict string.')
    if result == 'ANOMALY':
        pose = value.get('pose')
        if not isinstance(pose, dict): raise ValueError('ANOMALY requires the detection pose.')
        number(pose.get('x')); number(pose.get('y'))
    return value


def robot_name(robot, factory):
    # Detection AE may prefix the robot's canonical name with its factory letter.
    name = robot.strip()
    return name[1:] if name.startswith(factory+'site'+factory+'_') else name


def robot_fault_confirmed(db, source, suspect):
    robot = suspect['robot_id']
    scope = (source, robot, robot+'/status')
    # Include a fault that arrived before a delayed verdict and one that was
    # followed by recovery before this marker could be reconciled.
    latest = db.execute('''
        SELECT id FROM events
        WHERE source=? AND robot_id=? AND resource_path=?
        ORDER BY cse_created_at DESC, id DESC LIMIT 1
    ''', scope).fetchone()
    if latest is None:
        return False
    rows = db.execute('''
        SELECT content_json FROM events
        WHERE source=? AND robot_id=? AND resource_path=?
          AND (id=? OR cse_created_at>=?)
    ''', (*scope, latest['id'], suspect['reported_at']))
    for row in rows:
        try:
            value = content(row['content_json'])
        except (ValueError, RecursionError):
            continue
        if isinstance(value, dict) and primary_state(value.get('state')) == 'inspection_wait':
            return True
    return False


class AnalysisVerdict:
    def __init__(self, cfg, store, floor):
        self.cfg, self.store, self.floor = cfg, store, floor
        self.lock = threading.RLock(); self.stopping = threading.Event()
        self.connecting = False; self.connection_error = ''; self.receive_error = ''; self.verified_connection = None
        with store.connect() as db:
            db.executescript('''
              CREATE TABLE IF NOT EXISTS analysis_v_connections (
                key TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL, data_json TEXT NOT NULL);
              CREATE TABLE IF NOT EXISTS analysis_v_reports (
                connection_key TEXT NOT NULL, cin_ri TEXT NOT NULL, ct TEXT NOT NULL,
                raw_json TEXT NOT NULL, received_at TEXT NOT NULL, PRIMARY KEY(connection_key,cin_ri));
              CREATE TABLE IF NOT EXISTS analysis_v_suspects (
                connection_key TEXT NOT NULL, robot_id TEXT NOT NULL, data_json TEXT NOT NULL,
                state TEXT NOT NULL, PRIMARY KEY(connection_key,robot_id));
            ''')

    def target(self):
        cfg = self.floor.configuration()
        profile = validate_profile(cfg['profiles'][cfg['selected']])
        key = hashlib.sha256(dumps(['analysis-verdict',cfg['selected'],profile]).encode()).hexdigest()
        return cfg, profile, key

    def connection(self, key):
        with self.store.connect() as db:
            row = db.execute('SELECT data_json FROM analysis_v_connections WHERE key=?',(key,)).fetchone()
        return strict_json(row[0]) if row else None

    def put(self, conn):
        with self.store.connect() as db:
            db.execute('INSERT INTO analysis_v_connections VALUES (?,?,?) ON CONFLICT(key) DO UPDATE SET data_json=excluded.data_json',
                       (conn['key'],conn['token'],dumps(conn)))

    def current(self, conn):
        cfg, profile, key = self.target()
        return conn['key'] == key and conn.get('revision') == cfg['revision']

    def reconcile(self, conn):
        source = conn['profile']['url']
        floor = self.floor.snapshot()
        judge = floor.get('connection')
        with self.store.connect() as db:
            zones = []
            if judge and judge['profile']['url'] == source and floor['factory'] == conn['profile']['factory']:
                zones = [(strict_json(r['data_json']),r['state']) for r in db.execute("SELECT data_json,state FROM floor_j_zones WHERE connection_key=? AND state IN ('active','cleared')",(judge['key'],))]
            for row in db.execute("SELECT robot_id,data_json FROM analysis_v_suspects WHERE connection_key=? AND state='active'",(conn['key'],)).fetchall():
                suspect = strict_json(row['data_json'])
                if any(is_rectangle(zone) and rectangle_contains(zone,suspect['x'],suspect['y']) and
                       (state != 'cleared' or zone.get('reported_at','') >= suspect['reported_at']) for zone,state in zones):
                    state = 'confirmed'
                elif robot_fault_confirmed(db, source, suspect):
                    state = 'robot_fault'
                else:
                    continue
                db.execute("UPDATE analysis_v_suspects SET state=? WHERE connection_key=? AND robot_id=? AND state='active'",
                           (state,conn['key'],row['robot_id']))

    def snapshot(self):
        with self.lock:
            cfg, profile, key = self.target(); conn = self.connection(key)
            if conn and conn.get('revision') != cfg['revision']: conn = None
            suspects = []
            if conn:
                self.reconcile(conn)
                with self.store.connect() as db:
                    suspects = [strict_json(row[0]) for row in db.execute("SELECT data_json FROM analysis_v_suspects WHERE connection_key=? AND state='active'",(key,))]
            return {'selected':cfg['selected'],'factory':profile['factory'],'profile':profile,'connection':conn,
                    'suspects':suspects,'verdict_path':VERDICTS,'connection_error':self.connection_error,'receive_error':self.receive_error,'last_callback':getattr(self,'last_callback',None)}

    def set_parent(self, conn, parent):
        old = conn.get('parent_ri')
        if old is not None and old != parent:
            conn.update(previous_parent_ri=old,container_changed_at=utc_now(),
                        last_ct='',last_st=None,last_received=None,last_value=None,last_robot_id=None,
                        status='pending',receive_status='waiting',message='Detection container reset; waiting for new verdicts.')
            conn.pop('sub_ri',None)
            self.verified_connection = None
        conn['parent_ri'] = parent
        if not conn.get('last_received'): conn.setdefault('receive_status','waiting')

    def refresh_container(self, conn):
        # Empty latest or a new CIN parent can mean that the same path was recreated.
        profile = conn['profile']
        cnt = self.floor.rpc(profile,'GET',VERDICTS).get('m2m:cnt',{})
        if not isinstance(cnt,dict) or not isinstance(cnt.get('ri'),str):
            raise ValueError('Detection container unavailable.')
        cb = None
        if cnt['ri'] != conn.get('parent_ri'):
            cb = self.floor.rpc(profile,'GET').get('m2m:cb',{})
        with self.lock:
            current = self.connection(conn['key'])
            if not current or not self.current(conn) or not self.current(current): return None
            self.set_parent(current,cnt['ri'])
            if isinstance(cb,dict) and isinstance(cb.get('csi'),str): current['csi'] = cb['csi']
            self.put(current)
            return current

    def prepare(self):
        cfg, profile, key = self.target()
        cb = self.floor.rpc(profile,'GET').get('m2m:cb',{})
        if not isinstance(cb,dict) or cb.get('ty') != 5 or not isinstance(cb.get('csi'),str) or not cb['csi'].startswith('/'):
            raise ValueError('Analysis connection requires a CSEBase.')
        cnt = self.floor.rpc(profile,'GET',VERDICTS).get('m2m:cnt',{})
        if not isinstance(cnt,dict) or cnt.get('ty') != 3 or not isinstance(cnt.get('ri'),str):
            raise ValueError('Analysis verdict CNT must exist.')
        with self.lock:
            if self.target()[0]['revision'] != cfg['revision']: raise ValueError('CSE selection changed.')
            conn = self.connection(key) or {'key':key,'token':uuid.uuid4().hex,'last_ct':'','last_st':None,'status':'pending'}
            self.set_parent(conn,cnt['ri'])
            conn.update(profile=profile,revision=cfg['revision'],csi=cb['csi'])
            conn['callback'] = callback_url(self.cfg,conn,'/onem2m/analysis-verdict/')
            conn['sub_name'] = 'edgeway-analysis-'+conn['token'][:16]
            conn['sub_path'] = VERDICTS+'/'+conn['sub_name']; self.put(conn)
            return conn

    def subscribe(self, conn):
        with self.lock:
            if self.connecting or not self.current(conn): return
            self.connecting = True
            current = self.connection(conn['key']); current.update(status='connecting',message='Confirming detection subscription.'); self.put(current)
        try:
            profile = conn['profile']; key = conn['key']
            sub, delivery = ensure_subscription(self.floor.rpc,self.cfg,conn,VERDICTS,'/onem2m/analysis-verdict/')
            with self.lock:
                current = self.connection(key)
                if current['parent_ri'] != conn['parent_ri']: return
                current.update(**delivery,status='active',sub_ri=sub['ri'],message='Detection verdict subscription connected.'); self.put(current)
                self.verified_connection = (key,current['revision'])
        except Exception as exc:
            message = str(exc)
            cause = exc.__cause__
            if isinstance(cause,HTTPError):
                try:
                    body = cause.read(4096).decode(errors='replace').strip()
                    if body:
                        body = body.replace(conn['callback'],profile['notification_base']+'/onem2m/analysis-verdict/<token>')
                        message += ' | '+body[:600]
                except OSError: pass
            with self.lock:
                current = self.connection(conn['key']); current.update(status='error',message=message,delivery_attempts=conn.get('delivery_attempts',[])); self.put(current)
            if message != str(exc): raise ValueError(message) from exc
            raise
        finally:
            with self.lock: self.connecting = False

    def notification(self, token, origin, payload):
        with self.store.connect() as db:
            row = db.execute('SELECT data_json FROM analysis_v_connections WHERE token=?',(token,)).fetchone()
        if not row: raise PermissionError('Unknown analysis subscription.')
        conn = strict_json(row[0])
        if origin.rstrip('/') != conn['csi'].rstrip('/'): raise PermissionError('Wrong analysis notification origin.')
        sgn = payload.get('m2m:sgn') if isinstance(payload,dict) else None
        if not isinstance(sgn,dict): raise ValueError('Expected m2m:sgn.')
        if not self.current(conn): return
        if sgn.get('vrq') is True: return
        if sgn.get('sud') is True:
            with self.lock:
                current = self.connection(conn['key']); current.update(status='error',message='Analysis subscription deleted.'); self.put(current)
            return
        nev = sgn.get('nev')
        if not isinstance(nev,dict) or nev.get('net') != 3: raise ValueError('Expected a CIN creation notification.')
        name = urlsplit(conn['profile']['url']).path.rstrip('/').split('/')[-1]
        paths = {conn['sub_path'],'/'+conn['sub_path'],'/'+name+'/'+conn['sub_path'],conn['csi']+'/'+conn['sub_path']}
        if conn.get('sub_ri'): paths.update({conn['sub_ri'],'/'+conn['sub_ri'],conn['csi']+'/'+conn['sub_ri']})
        if sgn.get('sur') not in paths: raise ValueError('Wrong analysis subscription reference.')
        self.ingest(conn['key'],nev.get('rep',{}).get('m2m:cin'),payload)

    def ingest(self, key, cin, raw):
        if not isinstance(cin,dict) or cin.get('ty') != 4 or not isinstance(cin.get('ri'),str) or not isinstance(cin.get('ct'),str):
            raise ValueError('Expected a CIN with ri/ct/ty.')
        with self.lock:
            conn = self.connection(key)
            if not conn or not self.current(conn): return
        if cin.get('pi') != conn['parent_ri']:
            self.refresh_container(conn)
        with self.lock:
            conn = self.connection(key)
            if not conn or not self.current(conn) or cin.get('pi') != conn['parent_ri']: return
            value = verdict(cin.get('con'),conn['profile']['factory']); robot = robot_name(value['robot'],conn['profile']['factory']); st = cin.get('st')
            if type(st) is int and type(conn.get('last_st')) is int:
                if st < conn['last_st']: return
            elif conn.get('last_ct') and cin['ct'] < conn['last_ct']: return
            received = utc_now()
            with self.store.connect() as db:
                if db.execute('SELECT 1 FROM analysis_v_reports WHERE connection_key=? AND cin_ri=?',(key,cin['ri'])).fetchone(): return
                db.execute('INSERT INTO analysis_v_reports VALUES (?,?,?,?,?)',(key,cin['ri'],cin['ct'],dumps(raw),received))
                if value['verdict'] == 'ANOMALY':
                    point = {'id':cin['ri'],'robot_id':robot,'reported_robot':value['robot'],'site':value.get('site'),'x':value['pose']['x'],'y':value['pose']['y'],
                             'reported_at':cin['ct'],'received_at':received,'verdict':'ANOMALY'}
                    db.execute("INSERT INTO analysis_v_suspects VALUES (?,?,?,'active') ON CONFLICT(connection_key,robot_id) DO UPDATE SET data_json=excluded.data_json,state='active'",(key,robot,dumps(point)))
            conn.update(last_ct=cin['ct'],last_st=st if type(st) is int else None,last_received=received,receive_status='active',last_value=value,last_robot_id=robot); self.put(conn)
            self.reconcile(conn)

    def start(self):
        threading.Thread(target=self.receive,name='analysis-verdict-values',daemon=True).start()
        threading.Thread(target=self.follow,name='analysis-verdict-subscription',daemon=True).start()

    def receive(self):
        # GET reception never waits on SUB confirmation; callbacks share the same dedup path.
        while not self.stopping.is_set():
            started = time.monotonic()
            try:
                cfg, profile, key = self.target(); conn = self.connection(key)
                if not conn or conn.get('revision') != cfg['revision']: conn = self.prepare()
                try:
                    cin = self.floor.rpc(profile,'GET',VERDICTS+'/la').get('m2m:cin')
                    if cin: self.ingest(key,cin,{'retrieved':True,'m2m:cin':cin})
                except CSEError as exc:
                    if exc.status != 404: raise
                    # A real empty container is a waiting state, not a failed receive.
                    self.refresh_container(conn)
                self.receive_error = ''
                with self.lock:
                    current = self.connection(key)
                    if current and self.current(current):
                        current.update(last_poll_at=utc_now()); self.put(current); self.reconcile(current)
            except (ValueError,OSError,CSEError,sqlite3.Error) as exc:
                self.receive_error = str(exc)
            self.stopping.wait(max(.1,1-(time.monotonic()-started)))

    def follow(self):
        while not self.stopping.is_set():
            try:
                cfg, profile, key = self.target(); conn = self.connection(key)
                if conn and conn.get('revision') == cfg['revision'] and (conn.get('status') != 'active' or self.verified_connection != (key,cfg['revision'])): self.subscribe(conn)
                self.connection_error = ''
            except (ValueError,OSError,CSEError,sqlite3.Error) as exc:
                self.connection_error = str(exc)
                if self.stopping.wait(5): break
            self.stopping.wait(.5)
