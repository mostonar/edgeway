"""Judgement AE subscriptions, persistent floor zones and explicit zone-clear CINs."""
from datetime import datetime, timezone
import hashlib
import json
import math
from pathlib import Path
import re
import sqlite3
import ssl
import threading
import time
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import HTTPSHandler, ProxyHandler, Request, build_opener
import uuid

from core import CSEError, NoRedirect, dumps, strict_json, utc_now
from robot_state import primary_state
import factory_c_address
from subscription_delivery import callback_url, ensure_subscription

JUDGEMENT = 'JUDGE-AE-3/cnt-judgement'
REQUESTS = 'JUDGE-AE-3/cnt-request'
CONFIG_KEY = 'floor_judgement_config'


def number(value, minimum=-1e9, maximum=1e9):
    if type(value) not in (int, float) or not math.isfinite(value) or not minimum <= value <= maximum:
        raise ValueError('좌표는 범위 안의 유한한 숫자여야 합니다.')
    return value


def rectangle_bounds(zone):
    if not isinstance(zone,dict): raise ValueError('사각형 좌표가 필요합니다.')
    x1,y1,x2,y2=(number(zone.get(key)) for key in ('x1','y1','x2','y2'))
    if x1==x2 or y1==y2: raise ValueError('사각형의 가로와 세로 길이는 0보다 커야 합니다.')
    return min(x1,x2),min(y1,y2),max(x1,x2),max(y1,y2)


def rectangle_center(zone):
    x1,y1,x2,y2=rectangle_bounds(zone)
    return (x1+x2)/2,(y1+y2)/2


def rectangle_contains(zone,x,y):
    x1,y1,x2,y2=rectangle_bounds(zone)
    return x1<=x<=x2 and y1<=y<=y2


def is_rectangle(zone):
    try:rectangle_bounds(zone);return True
    except (ValueError,TypeError):return False


def content(value):
    for _ in range(2):
        if not isinstance(value, str): break
        value = strict_json(value)
    return value


def validate_zones(value):
    value = content(value)
    if not isinstance(value, dict) or not isinstance(value.get('zones'), list) or len(value['zones']) > 256:
        raise ValueError('con.zones에는 최대 256개 구역의 목록이 필요합니다.')
    zones, seen = [], set()
    for item in value['zones']:
        if not isinstance(item, dict): raise ValueError('구역 형식이 올바르지 않습니다.')
        data={key:number(item.get(key)) for key in ('x1','y1','x2','y2')}
        bounds=rectangle_bounds(data)
        key=dumps(['rectangle',*bounds])
        if key in seen:raise ValueError('같은 사각형이 중복되었습니다.')
        seen.add(key);zones.append({**data,'shape':'rectangle','key':key})
    return zones


def validate_profile(value):
    if not isinstance(value, dict): raise ValueError('연결 설정 형식이 올바르지 않습니다.')
    result = {}
    for key in ('url', 'notification_base'):
        url = value.get(key, '')
        if not isinstance(url, str) or len(url) > 1500: raise ValueError('연결 주소를 확인하세요.')
        url = url.strip().rstrip('/'); u = urlsplit(url)
        if u.scheme not in ('http', 'https') or not u.hostname or u.username or u.password or u.query or u.fragment or any(c.isspace() for c in url):
            raise ValueError('계정 정보나 쿼리가 없는 HTTP(S) 주소를 입력하세요.')
        result[key] = url
    if value.get('factory') not in ('A', 'B', 'C'): raise ValueError('공장을 선택하세요.')
    if type(value.get('verify_tls')) is not bool: raise ValueError('인증서 검증 설정을 확인하세요.')
    origin = value.get('origin', 'CAdmin')
    if not isinstance(origin, str) or not 1 <= len(origin) <= 200 or any(ord(c) < 33 or ord(c) > 126 for c in origin):
        raise ValueError('요청 Origin을 확인하세요.')
    result.update(factory=value['factory'], verify_tls=value['verify_tls'], origin=origin,
                  robot_length=number(value.get('robot_length'), .01, 100), robot_width=number(value.get('robot_width'), .01, 100))
    return result


class FloorJudgement:
    def __init__(self, cfg, store):
        self.cfg, self.store = cfg, store
        self.lock = threading.RLock(); self.connecting = False; self.connection_error = ''; self.receive_error = ''; self.verified_subscription = None
        self.stopping = threading.Event()
        with store.connect() as db:
            db.executescript('''
                CREATE TABLE IF NOT EXISTS floor_j_connections (
                  key TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL, data_json TEXT NOT NULL);
                CREATE TABLE IF NOT EXISTS floor_j_reports (
                  connection_key TEXT NOT NULL, cin_ri TEXT NOT NULL, ct TEXT NOT NULL,
                  raw_json TEXT NOT NULL, received_at TEXT NOT NULL, PRIMARY KEY(connection_key,cin_ri));
                CREATE TABLE IF NOT EXISTS floor_j_zones (
                  id TEXT PRIMARY KEY, connection_key TEXT NOT NULL, point_key TEXT NOT NULL,
                  data_json TEXT NOT NULL, state TEXT NOT NULL, seen_absent INTEGER NOT NULL DEFAULT 0,
                  UNIQUE(connection_key,point_key));
                CREATE TABLE IF NOT EXISTS floor_j_requests (
                  request_id TEXT PRIMARY KEY, zone_id TEXT NOT NULL, connection_key TEXT NOT NULL,
                  state TEXT NOT NULL, cause TEXT NOT NULL, robot_id TEXT NOT NULL,
                  body_json TEXT NOT NULL, url TEXT NOT NULL, message TEXT NOT NULL, created_at TEXT NOT NULL);
            ''')
            db.execute("UPDATE floor_j_requests SET state='unknown',message='서버 재시작 전 전송 결과를 확인하지 못했습니다. CNT 확인이 필요합니다.' WHERE state='sending'")

    def configuration(self):
        # The existing factory C selection is the only environment switch.
        chosen=factory_c_address.selection(self.cfg,self.store)
        selected='local' if chosen['preset']=='local' else 'mn'
        try:
            demo=strict_json((Path(self.cfg['config_path']).parent/'demo-command.json').read_text())
        except (OSError,ValueError):
            demo={}
        if not isinstance(demo,dict):raise ValueError('명령 연결 설정 형식을 확인하세요.')
        dimensions=self.cfg.get('floor_judgement',{})
        common={'factory':'C','origin':demo.get('origin',self.cfg['origin']),
                'verify_tls':demo.get('verify_tls',True) is not False,
                'robot_length':dimensions.get('robot_length',3.12),'robot_width':dimensions.get('robot_width',1.14)}
        remote_base=dimensions.get('mn_notification_base', self.cfg['notification_url'].split('/onem2m/notify',1)[0])
        choices=factory_c_address.presets(self.cfg)
        profiles={'local':{**common,'url':choices['local']['url'],'notification_base':'http://127.0.0.1:'+str(self.cfg['port'])}}
        if 'mep-c' in choices:
            profiles['mn']={**common,'url':choices['mep-c']['url'],'notification_base':remote_base}
        saved=self.store.setting(CONFIG_KEY,{})
        active=saved.get('active_key') if saved.get('revision')==chosen['revision'] and saved.get('selected')==selected and saved.get('profiles')==profiles else None
        return {'selected':selected,'revision':chosen['revision'],'active_key':active,'profiles':profiles}

    def connection(self, key):
        if not key: return None
        with self.store.connect() as db:
            row = db.execute('SELECT data_json FROM floor_j_connections WHERE key=?', (key,)).fetchone()
        return strict_json(row[0]) if row else None

    def put_connection(self, item):
        with self.store.connect() as db:
            db.execute('INSERT INTO floor_j_connections VALUES (?,?,?) ON CONFLICT(key) DO UPDATE SET data_json=excluded.data_json',
                       (item['key'], item['token'], dumps(item)))

    def snapshot(self):
        with self.lock:
            cfg = self.configuration(); conn = self.connection(cfg.get('active_key'))
            zones = []
            if conn:
                with self.store.connect() as db:
                    for row in db.execute("SELECT id,data_json,state FROM floor_j_zones WHERE connection_key=? AND state NOT IN ('cleared','cleaned')", (conn['key'],)):
                        zone = strict_json(row['data_json'])
                        if not is_rectangle(zone):continue  # Preserve legacy circle records, never invent a box for them.
                        request = db.execute('SELECT state,message FROM floor_j_requests WHERE zone_id=? ORDER BY rowid DESC LIMIT 1', (row['id'],)).fetchone()
                        zones.append({**zone, 'id': row['id'], 'state': row['state'],
                                      'request': dict(request) if request else None})
            history=[]
            if conn:
                with self.store.connect() as db:
                    history=[dict(r) for r in db.execute('SELECT cause,robot_id,state,message,created_at,body_json FROM floor_j_requests WHERE connection_key=? ORDER BY rowid DESC LIMIT 10',(conn['key'],))]
            profile = cfg['profiles'][cfg['selected']]
            return {'config': cfg, 'connection': conn, 'zones': zones, 'factory': profile['factory'],
                    'robot_margin': 0, 'geometry': 'rectangle',
                    'judgement_path': JUDGEMENT, 'request_path': REQUESTS, 'history': history, 'connection_error': self.connection_error, 'receive_error': self.receive_error, 'last_callback': getattr(self,'last_callback',None)}

    def rpc(self, profile, method, suffix='', body=None, ty=None):
        url = profile['url'] + ('/' + suffix.strip('/') if suffix else '')
        ri = 'edgeway-floor-' + uuid.uuid4().hex
        headers = {'Accept': 'application/json', 'X-M2M-Origin': profile['origin'], 'X-M2M-RVI': '3', 'X-M2M-RI': ri}
        raw = None
        if body is not None:
            headers['Content-Type'] = 'application/json' + (';ty='+str(ty) if ty else '')
            raw = dumps(body).encode()
        context = ssl.create_default_context()
        if not profile['verify_tls']:
            context.check_hostname=False;context.verify_mode=ssl.CERT_NONE
        opener = build_opener(ProxyHandler({}), NoRedirect(), HTTPSHandler(context=context))
        try:
            with opener.open(Request(url, method=method, headers=headers, data=raw), timeout=15) as response:
                payload = response.read(1048577)
                if len(payload)>1048576: raise ValueError('CSE 응답이 너무 큽니다.')
                rsc = response.headers.get('X-M2M-RSC', '')
                expected = {'POST': ('201','2001'), 'PUT': ('200','2004'), 'DELETE': ('200','2002')}.get(method, ('200','2000'))
                if (str(response.status),rsc)!=expected or response.headers.get('X-M2M-RI')!=ri:
                    raise ValueError('CSE가 요청 성공을 확인하지 않았습니다.')
                result = strict_json(payload)
                if not isinstance(result, dict): raise ValueError('CSE JSON 응답 형식이 올바르지 않습니다.')
                return result
        except HTTPError as exc:
            raise CSEError(method+' '+(suffix or 'CSEBase')+': CSE HTTP '+str(exc.code)+' / RSC '+exc.headers.get('X-M2M-RSC','-'), exc.code, exc.headers.get('X-M2M-RSC')) from exc
        except (OSError,ValueError) as exc:
            raise ValueError(method+' '+(suffix or 'CSEBase')+': '+str(exc)) from exc

    def prepare(self):
        cfg=self.configuration(); profile=validate_profile(cfg['profiles'][cfg['selected']])
        cb=self.rpc(profile,'GET').get('m2m:cb',{})
        if not isinstance(cb,dict) or cb.get('ty')!=5 or not isinstance(cb.get('csi'),str) or not cb['csi'].startswith('/'):
            raise ValueError('CSEBase 및 csi 확인에 실패했습니다.')
        cnt=self.rpc(profile,'GET',JUDGEMENT).get('m2m:cnt',{})
        if not isinstance(cnt,dict) or cnt.get('ty')!=3 or not isinstance(cnt.get('ri'),str):
            raise ValueError('판정 CNT가 먼저 준비되어 있어야 합니다.')
        key=hashlib.sha256(dumps([cfg['selected'],profile]).encode()).hexdigest()
        with self.lock:
            if self.configuration()['revision']!=cfg['revision']: raise ValueError('연결 설정이 변경됐습니다.')
            conn=self.connection(key) or {'key':key,'token':uuid.uuid4().hex,'profile':profile,'last_ct':'','last_st':None,'status':'pending'}
            if conn.get('parent_ri') and conn['parent_ri']!=cnt['ri']:
                conn.update(previous_parent_ri=conn['parent_ri'],container_changed_at=utc_now(),last_ct='',last_st=None,
                            last_received=None,last_value=None,status='pending',receive_status='waiting')
                conn.pop('sub_ri',None);self.verified_subscription=None
                # A new container is a new generation; old clear receipts remain audited.
                with self.store.connect() as db:
                    db.execute("UPDATE floor_j_zones SET seen_absent=1 WHERE connection_key=? AND state='cleared'",(key,))
            conn.update(csi=cb['csi'],parent_ri=cnt['ri'],geometry='rectangle')
            conn.setdefault('receive_status','waiting')
            conn['callback']=callback_url(self.cfg,conn,'/onem2m/floor-judgement/')
            conn['sub_name']='edgeway-floor-'+conn['token'][:16]
            conn['sub_path']=JUDGEMENT+'/'+conn['sub_name'];self.put_connection(conn)
            cfg['active_key']=key;self.store.put_setting(CONFIG_KEY,cfg)
            return conn

    def connect(self):
        cfg=self.configuration();conn=self.connection(cfg.get('active_key'))
        if not conn or conn.get('geometry')!='rectangle':conn=self.prepare()
        profile=conn['profile'];key=conn['key']
        with self.lock:
            if self.connecting:return
            self.connecting=True
            current=self.connection(key);current.update(status='connecting',message='구독 연결 중');self.put_connection(current)
        try:
            sub,delivery=ensure_subscription(self.rpc,self.cfg,conn,JUDGEMENT,'/onem2m/floor-judgement/')
            with self.lock:
                current=self.connection(key)
                if current['parent_ri']!=conn['parent_ri']:return
                current.update(**delivery,status='active',sub_ri=sub['ri'],message='구독 연결됨. 새 판정 알림을 기다립니다.');self.put_connection(current)
                self.verified_subscription=(key,current['parent_ri'])
            return self.snapshot()
        except Exception as exc:
            with self.lock:
                current=self.connection(key);current.update(status='error',message=str(exc),delivery_attempts=conn.get('delivery_attempts',[]));self.put_connection(current)
            raise
        finally:
            with self.lock:self.connecting=False

    def notification(self, token, origin, payload):
        with self.store.connect() as db:
            row=db.execute('SELECT key,data_json FROM floor_j_connections WHERE token=?',(token,)).fetchone()
        if not row: raise PermissionError('알 수 없는 구독 알림입니다.')
        conn=strict_json(row['data_json'])
        if origin.rstrip('/')!=conn['csi'].rstrip('/'): raise PermissionError('구독 CSE와 알림 출처가 다릅니다.')
        sgn=payload.get('m2m:sgn') if isinstance(payload,dict) else None
        if not isinstance(sgn,dict): raise ValueError('m2m:sgn이 필요합니다.')
        if self.configuration().get('active_key')!=row['key']: return
        if sgn.get('vrq') is True:return
        if sgn.get('sud') is True:
            with self.lock:
                conn=self.connection(row['key']);conn.update(status='error',message='원격 구독이 삭제됐습니다. 다시 연결하세요.');self.put_connection(conn)
            return
        nev=sgn.get('nev')
        if not isinstance(nev,dict) or nev.get('net')!=3:raise ValueError('CIN 생성 알림만 처리합니다.')
        sur=sgn.get('sur');base_name=urlsplit(conn['profile']['url']).path.rstrip('/').split('/')[-1]
        paths={conn['sub_path'],'/'+conn['sub_path'],'/'+base_name+'/'+conn['sub_path'],conn['csi']+'/'+conn['sub_path']}
        if conn.get('sub_ri'):paths.update({conn['sub_ri'],'/'+conn['sub_ri'],conn['csi']+'/'+conn['sub_ri']})
        if not isinstance(sur,str) or sur not in paths:raise ValueError('구독 참조가 일치하지 않습니다.')
        cin=nev.get('rep',{}).get('m2m:cin')
        self.ingest(row['key'],cin,payload)

    def ingest(self, key, cin, raw):
        if not isinstance(cin,dict) or cin.get('ty')!=4 or not isinstance(cin.get('ri'),str) or not isinstance(cin.get('ct'),str):
            raise ValueError('유효한 CIN ri/ct/ty가 필요합니다.')
        zones=validate_zones(cin.get('con'))
        with self.lock:
            conn=self.connection(key)
            if not conn or self.configuration().get('active_key')!=key:return
        if cin.get('pi')!=conn['parent_ri']:self.prepare()
        with self.lock:
            conn=self.connection(key)
            if not conn or self.configuration().get('active_key')!=key or cin.get('pi')!=conn['parent_ri']:return
            st=cin.get('st')
            if type(st) is int and type(conn.get('last_st')) is int:
                if st<conn['last_st']:return
            elif conn.get('last_ct') and cin['ct']<conn['last_ct']:return
            with self.store.connect() as db:
                if db.execute('SELECT 1 FROM floor_j_reports WHERE connection_key=? AND cin_ri=?',(key,cin['ri'])).fetchone():return
                db.execute('INSERT INTO floor_j_reports VALUES (?,?,?,?,?)',(key,cin['ri'],cin['ct'],dumps(raw),utc_now()))
                present={z['key'] for z in zones}
                # Missing zones never clear active marks. Absence only rearms a previously cleared occurrence.
                for row in db.execute("SELECT id,point_key FROM floor_j_zones WHERE connection_key=? AND state='cleared'",(key,)).fetchall():
                    if row['point_key'] not in present:db.execute('UPDATE floor_j_zones SET seen_absent=1 WHERE id=?',(row['id'],))
                for zone in zones:
                    old=db.execute('SELECT * FROM floor_j_zones WHERE connection_key=? AND point_key=?',(key,zone['key'])).fetchone()
                    if old and old['state']=='cleared' and not old['seen_absent']:continue
                    data={k:zone[k] for k in ('x1','y1','x2','y2','shape')};data['reported_at']=cin['ct']
                    if old and old['state']=='cleaned':
                        db.execute("UPDATE floor_j_zones SET data_json=?,state='active' WHERE id=?",(dumps(data),old['id']))
                    elif old and old['state']!='cleared':
                        db.execute('UPDATE floor_j_zones SET data_json=? WHERE id=?',(dumps(data),old['id']))
                    else:
                        if old:db.execute('DELETE FROM floor_j_zones WHERE id=?',(old['id'],))
                        db.execute('INSERT INTO floor_j_zones VALUES (?,?,?,?,?,0)',(uuid.uuid4().hex,key,zone['key'],dumps(data),'active'))
            conn.update(last_ct=cin['ct'],last_st=st if type(st) is int else None,last_received=utc_now(),receive_status='active',last_value=content(cin.get('con')))
            self.put_connection(conn)

    def clear(self, data, cause='manual', robot_id=''):
        request_id=data.get('request_id')
        if not isinstance(request_id,str) or not re.fullmatch('[a-f0-9]{32}',request_id):raise ValueError('유효한 요청 ID가 필요합니다.')
        if data.get('confirm') is not True:raise ValueError('해제 요청 확인이 필요합니다.')
        with self.lock:
            with self.store.connect() as db:
                prior=db.execute('SELECT * FROM floor_j_requests WHERE request_id=?',(request_id,)).fetchone()
                if prior:
                    if prior['zone_id']!=data.get('zone_id') or prior['connection_key']!=data.get('connection_key'):raise ValueError('다른 구역의 요청 ID입니다.')
                    return dict(prior)
                cfg=self.configuration();conn=self.connection(cfg.get('active_key'))
                if not conn or (conn['status']!='active' and conn.get('receive_status')!='active') or data.get('connection_key')!=conn['key']:raise ValueError('활성 연결이 변경됐습니다. 다시 확인하세요.')
                row=db.execute("SELECT * FROM floor_j_zones WHERE id=? AND connection_key=? AND state='active'",(data.get('zone_id'),conn['key'])).fetchone()
                if not row:raise ValueError('구역이 이미 해제됐거나 다른 요청이 진행 중입니다.')
                previous=db.execute("SELECT * FROM floor_j_requests WHERE zone_id=? AND state IN ('sending','sent','unknown') ORDER BY rowid DESC LIMIT 1",(row['id'],)).fetchone()
                if previous:return dict(previous)
                zone=strict_json(row['data_json']);x,y=rectangle_center(zone)
                body={'m2m:cin':{'con':{'type':'zone-clear','x':x,'y':y}}}
                url=conn['profile']['url']+'/'+REQUESTS
                db.execute('INSERT INTO floor_j_requests VALUES (?,?,?,?,?,?,?,?,?,?)',(request_id,row['id'],conn['key'],'sending',cause,robot_id,dumps(body),url,'해제 요청 전송 중',utc_now()))
            # The persistent sending row blocks connection switching while the network request is in flight.
        state,message='unknown','전송 결과 불명확: 요청 CNT를 확인하기 전에는 재전송하지 않습니다.'
        try:
            cin=self.rpc(conn['profile'],'POST',REQUESTS,body,4).get('m2m:cin',{})
            if not isinstance(cin,dict) or cin.get('ty')!=4 or not isinstance(cin.get('ri'),str):raise ValueError('CIN 저장 응답이 올바르지 않습니다.')
            state,message='sent','해제 요청이 CSE에 저장됐습니다. 실제 현장 조치 완료를 뜻하지는 않습니다.'
        except CSEError as exc:
            if 300<=exc.status<500:state,message='failed',str(exc)
        except (OSError,ValueError,TypeError):pass
        with self.lock,self.store.connect() as db:
            db.execute('UPDATE floor_j_requests SET state=?,message=? WHERE request_id=?',(state,message,request_id))
            if state=='sent':db.execute("UPDATE floor_j_zones SET state='cleared',seen_absent=0 WHERE id=?",(row['id'],))
            return dict(db.execute('SELECT * FROM floor_j_requests WHERE request_id=?',(request_id,)).fetchone())

    def start(self):
        threading.Thread(target=self.monitor,name='floor-robot-fault',daemon=True).start()
        threading.Thread(target=self.receive_latest,name='floor-judgement-values',daemon=True).start()
        threading.Thread(target=self.follow_connection,name='floor-subscription',daemon=True).start()

    def receive_latest(self):
        # Continuous GET reception is independent of subscription confirmation/retry.
        while not self.stopping.is_set():
            started=time.monotonic()
            try:
                cfg=self.configuration();conn=self.connection(cfg.get('active_key'))
                if not conn or conn.get('geometry')!='rectangle':conn=self.prepare()
                try:
                    cin=self.rpc(conn['profile'],'GET',JUDGEMENT+'/la').get('m2m:cin')
                    if cin:
                        latest=content(cin.get('con'));items=latest.get('zones') if isinstance(latest,dict) else None
                        legacy=isinstance(items,list) and bool(items) and all(isinstance(item,dict) and 'r' in item and 'x1' not in item for item in items)
                        if not legacy:self.ingest(conn['key'],cin,{'retrieved':True,'m2m:cin':cin})
                except CSEError as exc:
                    if exc.status!=404:raise
                    self.prepare()
                self.receive_error=''
                with self.lock:
                    current=self.connection(conn['key'])
                    if current and self.configuration().get('active_key')==current['key']:
                        current.update(last_poll_at=utc_now());self.put_connection(current)
            except (ValueError,OSError,CSEError,sqlite3.Error) as exc:
                self.receive_error=str(exc)
            self.stopping.wait(max(.1,1-(time.monotonic()-started)))

    def follow_connection(self):
        # No extra UI procedure: subscribe to the already-selected CSE and follow changes.
        while not self.stopping.is_set():
            try:
                cfg=self.configuration();conn=self.connection(cfg.get('active_key'))
                if not conn or conn.get('status')!='active' or conn.get('geometry')!='rectangle' or self.verified_subscription!=(conn['key'],conn.get('parent_ri')):
                    self.connect()
                self.connection_error=''
            except (ValueError,OSError,CSEError,sqlite3.Error) as exc:
                self.connection_error=str(exc)
                if self.stopping.wait(5):break
            self.stopping.wait(.5)

    def monitor(self):
        # Initial baseline excludes old robot faults from before this feature/connection was enabled.
        cursor=self.store.setting('floor_judgement_robot_cursor',None)
        if cursor is None:
            with self.store.connect() as db:cursor=db.execute('SELECT COALESCE(MAX(id),0) FROM events').fetchone()[0]
        while not self.stopping.wait(.5):
            try:
                with self.store.connect() as db:rows=db.execute('SELECT id,source,robot_id,resource_path,content_json FROM events WHERE id>? ORDER BY id LIMIT 100',(cursor,)).fetchall()
                for row in rows:
                    self.robot_fault(dict(row));cursor=row['id']
                    self.store.put_setting('floor_judgement_robot_cursor',cursor)
            except (ValueError,OSError,sqlite3.Error):self.stopping.wait(1)

    def robot_fault(self, event):
        snap=self.snapshot();conn=snap['connection']
        if not conn or (conn['status']!='active' and conn.get('receive_status')!='active') or event.get('source','').rstrip('/')!=conn['profile']['url']:return
        factory=conn['profile']['factory']
        if not re.fullmatch('site'+factory+'_AMR_[1-9][0-9]*',event.get('robot_id','')):return
        if event.get('resource_path')!=event['robot_id']+'/status':return
        payload=content(event.get('content_json'))
        if not isinstance(payload,dict) or primary_state(payload.get('state'))!='inspection_wait':return
        pose=payload.get('pose')
        if not isinstance(pose,dict):return
        try:x,y=number(pose.get('x')),number(pose.get('y'))
        except ValueError:return
        for zone in snap['zones']:
            if zone.get('request'):continue
            if rectangle_contains(zone,x,y):
                try:self.clear({'zone_id':zone['id'],'connection_key':conn['key'],'request_id':uuid.uuid4().hex,'confirm':True},'robot_fault',event['robot_id'])
                except ValueError:pass
