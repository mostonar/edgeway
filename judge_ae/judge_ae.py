# JUDGE-AE-<n>: anomaly judgement AE for one site (oneM2M). Run one per site (A=1, B=2, C=3).
#
# Usage
#   python judge_ae.py --site {A,B,C} --cse <CSE base URL incl. CSE name> [--robots <AE>,<AE>,...]
#                      [--notify <URL>] [--port 9090]
#                      [--insecure | --ca <CA file>] [--check-s 1] [--grace-s 30] [--timeout-s 600]
#                      [--zone-margin 2] [--zone-min 5] [--alive-s 5] [--iot <MEC 033 URL>]
#   --site  site to judge (default C); sets the AE name, detection AE and robots below.
#   --robots  robot AE names (default: siteA_AMR_1..4 / siteB_AMR_1..2 / siteC_AMR_1..2).
#   --notify  URL where the CSE delivers notifications (CSE must reach it). Omit to use a pollingChannel.
#   --insecure / --ca  for an https CSE with a self-signed certificate.
#   --alive-s  CSE liveness check interval. Writes are held while the CSE is down and resent when it is back.
#   --iot  MEP's MEC 033 registered_iot_platforms URL. While the CSE is down, ask the MEP for the current CSE
#          and move to it if it changed (AE, containers, subscriptions are created again there).
#   Example: python judge_ae.py --site C --cse https://<MEC_HOST>/<SANDBOX>/mep-c/meep-acme-mn-cse/acme-cse-mn-mep-c-<SANDBOX> --insecure
#
# Resources (n = site number)
#   subscribes  ANALYSIS-AE-<n>/cnt-verdict    anomaly reports from the detection AE
#   subscribes  JUDGE-AE-<n>/cnt-request       dashboard request  {"type": "zone-clear", "x": 60.0, "y": 14.0}
#   writes      JUDGE-AE-<n>/cnt-judgement     no-go zones  {"zones": [{"x1", "y1", "x2", "y2"}, ...]}
#   writes      <robot>/command                verdict  {"type": "verdict", "case_id", "stage": "suspect"|"final"|"closed",
#                                                        "site", "target", "pos", "cause": "robot"|"floor", "zone"}
#   ACP acp_judgement: JUDGE-AE-<n> + CAdmin only (the dashboard uses CAdmin)

import argparse, collections, json, math, queue, signal, threading, time, urllib.parse, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import requests

SITES = {'A': (1, 4), 'B': (2, 2), 'C': (3, 2)}    # site -> (number, robot count)
REQ_CNT = 'cnt-request'
AE_NAME = ORIGIN = SITE = SOURCE = SUB_RN = REQUEST = SUB_REQ_RN = None
ROBOTS = []
PCH_RN = 'pch'
PCH_WAIT_S = 70.0
CNT = 'cnt-judgement'
ACP_RN = 'acp_judgement'
NEAR_M = 3.0
CHECK_S = 1.0
GRACE_S = 30.0
CASE_TIMEOUT_S = 600.0
ZONE_MIN = 5.0
ZONE_MARGIN = 2.0
ALIVE_S = 5.0
BACKLOG_MAX = 500

ev = queue.Queue()


def configure(site, robots=None):
    global AE_NAME, ORIGIN, SITE, ROBOTS, SOURCE, SUB_RN, REQUEST, SUB_REQ_RN
    n, count = SITES[site]
    AE_NAME = f'JUDGE-AE-{n}'
    ORIGIN = 'C' + AE_NAME
    SITE = f'site{site}'
    ROBOTS = robots or [f'{SITE}_AMR_{i}' for i in range(1, count + 1)]
    SOURCE = f'ANALYSIS-AE-{n}/cnt-verdict'
    SUB_RN = f'sub_judge_{n}'
    REQUEST = f'{AE_NAME}/{REQ_CNT}'
    SUB_REQ_RN = f'sub_judge_req_{n}'


def log(s, hl=False):
    line = time.strftime('%H:%M:%S ') + s
    print(f'\033[33m{line}\033[0m' if hl else line, flush=True)   # hl: key events in yellow


class CSE:
    def __init__(self, base, verify=True):
        self.base = base.rstrip('/')
        self.http = requests.Session()
        self.verify = verify
        self.gen = 0                                   # +1 on every CSE move
        self.alive, self.ready = threading.Event(), threading.Event()
        self.alive.set()
        self.wlock = threading.Lock()
        self.backlog = collections.deque(maxlen=BACKLOG_MAX)   # CIN writes not yet delivered, oldest first
        self.holding = False
        if verify is False:
            import urllib3
            urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    def req(self, method, path='', ty=None, body=None, timeout=5):
        h = {'X-M2M-Origin': ORIGIN, 'X-M2M-RI': uuid.uuid4().hex[:12], 'X-M2M-RVI': '3', 'Accept': 'application/json'}
        if ty:
            h['Content-Type'] = f'application/json;ty={ty}'
        elif body is not None:
            h['Content-Type'] = 'application/json'
        r = self.http.request(method, f'{self.base}/{path}'.rstrip('/'), headers=h,
                              data=json.dumps(body) if body is not None else None, timeout=timeout,
                              verify=self.verify)
        try:
            return r.status_code, r.json()
        except ValueError:
            return r.status_code, {}

    def cin(self, path, con):
        item = (path, con)
        with self.wlock:
            self.backlog.append(item)
            return self.flush(item) if self.alive.is_set() else False

    def flush(self, mine=None):
        """Send held writes in order (caller holds wlock). Stops at the first one the CSE cannot take."""
        ok = False
        while self.backlog:
            item = self.backlog[0]
            try:
                c, _ = self.req('POST', item[0], 4, {'m2m:cin': {'cnf': 'application/json:0', 'con': json.dumps(item[1])}})
            except requests.RequestException:
                c = None
            if c is None or c >= 500:
                self.alive.clear()
                if not self.holding:
                    log(f'[WARN] CSE not reachable -> holding writes for resend ({len(self.backlog)})')
                self.holding = True
                return False
            if c != 201:
                log(f'[WARN] write to {item[0]} failed ({c})')
            if item is mine:
                ok = c == 201
            self.backlog.popleft()
        if self.holding:
            log('Held writes resent')
            self.holding = False
        return ok

    def la(self, path):
        try:
            c, js = self.req('GET', f'{path}/la')
        except requests.RequestException:
            return None, None
        if c != 200:
            return None, None
        cin = js.get('m2m:cin', {})
        con = cin.get('con')
        try:
            con = json.loads(con) if isinstance(con, str) else con
        except ValueError:
            pass
        return cin.get('ri'), con


def setup_ae(cse, notify):
    upd = {'poa': [notify], 'rr': True} if notify else {'rr': False}
    ae = {'rn': AE_NAME, 'api': 'Nedgeway.judge', 'srv': ['3'], **upd}
    c, _ = cse.req('POST', '', 2, {'m2m:ae': ae})
    if c == 409 or (c >= 400 and cse.req('GET', AE_NAME)[0] == 200):
        c, _ = cse.req('PUT', AE_NAME, body={'m2m:ae': upd})
    elif c != 201:
        raise RuntimeError(f'AE registration failed ({c})')


def setup(cse, notify):
    nu = notify or ORIGIN

    acr = {'acr': [{'acor': [ORIGIN, 'CAdmin'], 'acop': 63}]}
    c, js = cse.req('POST', AE_NAME, 1, {'m2m:acp': {'rn': ACP_RN, 'pv': acr, 'pvs': {'acr': [acr['acr'][0]]}}})
    if c == 409:
        cse.req('PUT', f'{AE_NAME}/{ACP_RN}', body={'m2m:acp': {'pv': acr}})
        c, js = cse.req('GET', f'{AE_NAME}/{ACP_RN}')
    acp_ri = js.get('m2m:acp', {}).get('ri')
    for rn in (CNT, REQ_CNT):
        c, _ = cse.req('POST', AE_NAME, 3, {'m2m:cnt': {'rn': rn, 'mni': 200, 'acpi': [acp_ri]}})
        if c == 409:
            cse.req('PUT', f'{AE_NAME}/{rn}', body={'m2m:cnt': {'acpi': [acp_ri]}})
        elif c != 201:
            raise RuntimeError(f'{rn} creation failed ({c})')

    sub = {'rn': SUB_RN, 'nu': [nu], 'nct': 1, 'enc': {'net': [3]}}
    c, js = cse.req('POST', SOURCE, 23, {'m2m:sub': sub}, timeout=15)
    if c == 409:
        c, js = cse.req('PUT', f'{SOURCE}/{SUB_RN}', body={'m2m:sub': {'nu': [nu]}}, timeout=15)
    if c not in (200, 201, 204):
        why = ('CSE cannot reach --notify, run without --notify (pollingChannel)' if 'Verification' in str(js)
               else f'check that the detection AE ACP allows {ORIGIN}')
        raise RuntimeError(f'subscription to {SOURCE} failed ({c}): {str(js)[:300]}  <- {why}')
    log(f'Subscribed: {SOURCE} (anomaly reports)')
    sub = {'rn': SUB_REQ_RN, 'nu': [nu], 'nct': 1, 'enc': {'net': [3]}}
    c, js = cse.req('POST', REQUEST, 23, {'m2m:sub': sub}, timeout=15)
    if c == 409:
        c, js = cse.req('PUT', f'{REQUEST}/{SUB_REQ_RN}', body={'m2m:sub': {'nu': [nu]}}, timeout=15)
    if c not in (200, 201, 204):
        raise RuntimeError(f'subscription to {REQUEST} failed ({c}): {str(js)[:300]}')
    log(f'Subscribed: {REQUEST} (dashboard requests)')


def find(o, key):
    if isinstance(o, dict):
        for k, v in o.items():
            if k == key:
                yield v
            else:
                yield from find(v, key)
    elif isinstance(o, list):
        for v in o:
            yield from find(v, key)


def on_notify(body):
    for sgn in find(body, 'm2m:sgn'):
        if sgn.get('sud'):
            log('[WARN] subscription deleted by CSE (restart to subscribe again)')
        for cin in find(sgn.get('nev', {}), 'm2m:cin'):
            con = cin.get('con')
            try:
                con = json.loads(con) if isinstance(con, str) else con
            except ValueError:
                continue
            ev.put(con)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        try:
            body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or b'{}')
        except ValueError:
            body = {}
        on_notify(body)
        self.send_response(200)
        self.send_header('X-M2M-RSC', '2000')
        self.send_header('X-M2M-RI', self.headers.get('X-M2M-RI', ''))
        self.send_header('Content-Length', '0')
        self.end_headers()


def find_pch(cse):
    if cse.req('GET', f'{AE_NAME}/{PCH_RN}')[0] == 200:
        return f'{AE_NAME}/{PCH_RN}'
    c, js = cse.req('GET', f'{AE_NAME}?fu=1&ty=15')
    uril = js.get('m2m:uril') or []
    uril = uril if isinstance(uril, list) else uril.split()
    if c == 200 and uril:
        p = f"{AE_NAME}/{uril[0].rstrip('/').split('/')[-1]}"
        if cse.req('GET', p)[0] != 200:
            p = uril[0].rstrip('/').split('/')[-1]
        return p
    c, js = cse.req('POST', AE_NAME, 15, {'m2m:pch': {'rn': PCH_RN}})
    if c != 201:
        raise RuntimeError(f'pollingChannel creation failed ({c}): {str(js)[:200]}')
    return f'{AE_NAME}/{PCH_RN}'


def pch_loop(cse):
    path, gen = None, cse.gen
    while True:
        if gen != cse.gen:
            path, gen = None, cse.gen
        try:
            if path is None:
                path = find_pch(cse)
                cse.ready.set()
            c, js = cse.req('GET', f'{path}/pcu', timeout=PCH_WAIT_S)
            if c == 200:
                on_notify(js)
                for rqp in find(js, 'm2m:rqp') or [js]:
                    rqi = rqp.get('rqi') if isinstance(rqp, dict) else None
                    if rqi:
                        cse.req('POST', f'{path}/pcu', body={'m2m:rsp': {'rsc': 2000, 'rqi': rqi, 'rvi': '3', 'fr': ORIGIN}})
            elif c == 404:
                path = None
            elif c not in (204, 408, 504):
                time.sleep(2)
        except requests.exceptions.Timeout:
            pass
        except Exception:
            path = None
            time.sleep(5)


def mep_cse(cse, iot):
    """MEC 033 registered_iot_platforms -> (CSE-ID, CSE URL). Picks customServicesTransportInfo whose id is a CSE-ID.
    The MEP reports an internal host, so the scheme and host of the --iot URL are used instead."""
    r = cse.http.get(iot, headers={'Accept': 'application/json'}, timeout=3, verify=cse.verify)
    r.raise_for_status()
    q = urllib.parse.urlsplit(iot)
    found = []
    for p in r.json() or []:
        if p.get('enabled') is False:
            continue
        for ti in p.get('customServicesTransportInfo') or []:
            tid, name = str(ti.get('id') or ''), ti.get('name')
            uris = (ti.get('endpoint') or {}).get('uris') or []
            if tid.startswith('/') and name and uris:
                u = urllib.parse.urlsplit(uris[0].rstrip('/'))._replace(scheme=q.scheme, netloc=q.netloc)
                found.append((tid, f'{urllib.parse.urlunsplit(u)}/{name}'))
    if not found:
        raise RuntimeError('no CSE in the MEC 033 response')
    return next((f for f in found if f[1] == cse.base), found[0])


def sync(cse, notify):
    setup_ae(cse, notify)
    if not notify and not cse.ready.wait(15):
        raise RuntimeError('pollingChannel not ready')
    setup(cse, notify)


def watch(cse, notify, iot):
    """Every ALIVE_S: check the CSE. Down -> hold writes (and ask the MEP if --iot). Back or moved -> restore, resend."""
    up, resync, said = True, False, None

    def say(s):
        nonlocal said
        if s != said:
            log(s, hl=s.startswith('MEP: CSE moved'))
            said = s

    while True:
        time.sleep(ALIVE_S)
        try:
            c = cse.req('GET', AE_NAME, timeout=3)[0]
        except requests.RequestException:
            c = None
        if c is not None and c < 500:
            if not up:
                say(f'CSE reachable ({cse.base})')
                up, resync = True, True
            if c == 404:
                resync = True
            if resync:
                try:
                    sync(cse, notify)
                    resync = False
                    say('AE, containers and subscriptions restored')
                except Exception as e:
                    say(f'[WARN] restore failed, retrying: {e}')
                    continue
            cse.alive.set()
            with cse.wlock:
                if cse.backlog:
                    cse.flush()
            continue
        if up:
            say(f'[WARN] CSE not responding ({c or "no response"}) -> holding writes')
            up = False
        cse.alive.clear()
        if not iot:
            continue
        try:
            tid, url = mep_cse(cse, iot)
        except Exception as e:
            say(f'[WARN] MEP not responding -> network down, waiting ({e})')
            continue
        if url == cse.base:
            say(f'MEP: CSE unchanged ({tid}) -> retrying the same CSE')
            continue
        say(f'MEP: CSE moved {cse.base} -> {url} ({tid}) -> registering there')
        cse.base = url
        cse.ready.clear()
        cse.gen += 1
        resync = True


def dist(a, b):
    return math.hypot(a[0] - b[0], a[1] - b[1])


def closest(tel, target):
    try:
        pts = [(x, y) for x, y in zip(tel['pose']['x'], tel['pose']['y']) if x is not None and y is not None]
    except (KeyError, TypeError):
        return None, None
    if not pts:
        return None, None
    p = min(pts, key=lambda q: dist(q, target))
    return p, dist(p, target)


def judge(cse):
    case = None
    zones = []
    muted = {}
    publish_zones(cse, zones)
    while True:
        try:
            con = ev.get(timeout=1.0)
        except queue.Empty:
            con = None
        now = time.time()

        if con and con.get('type') == 'zone-clear':
            try:
                pt = (float(con['x']), float(con['y']))
            except (KeyError, TypeError, ValueError):
                pt = None
            z = next((z for z in zones if pt and inside(pt, z)), None)
            if z:
                zones.remove(z)
                log(f"Zone x {z['x1']}~{z['x2']}, y {z['y1']}~{z['y2']} cleared by dashboard -> robots notified")
                for r in ROBOTS:
                    cse.cin(f'{r}/command', {'type': 'verdict', 'stage': 'closed', 'site': SITE,
                                             'cause': 'floor', 'zone': z})
                publish_zones(cse, zones)
            con = None

        if con and con.get('type') == 'amr-verdict' and con.get('verdict') == 'ANOMALY' \
                and con.get('site', SITE) == SITE:
            robot = str(con.get('robot', ''))
            name = robot[1:] if robot.startswith('C') else robot
            p = con.get('pose') or {}
            pos = (p.get('x'), p.get('y'))
            pos = tuple(float(v) for v in pos) if None not in pos else pos
            if name not in ROBOTS or None in pos:
                pass
            elif name in muted and not cleared(cse, name, muted):
                pass
            elif any(inside(pos, z) for z in zones):
                pass
            elif case is None:
                case = {'id': 'case-' + uuid.uuid4().hex[:6], 'target': name, 'pos': pos, 't0': now,
                        'verifiers': [r for r in ROBOTS if r != name], 'next_check': now + CHECK_S,
                        'near_since': None}
                log(f"[{case['id']}] Anomaly reported: {name} at ({pos[0]:.1f}, {pos[1]:.1f}) -> SUSPECT, "
                    f"cross-check by {', '.join(case['verifiers'])}", hl=True)
                send(cse, case, 'suspect')
            elif name in case['verifiers'] and dist(pos, case['pos']) <= NEAR_M:
                log(f"[{case['id']}] {name} also anomalous at ({pos[0]:.1f}, {pos[1]:.1f}) -> FINAL: FLOOR anomaly", hl=True)
                case['zone'] = merge_zone(case['pos'], pos)
                zones.append(case['zone'])
                z = case['zone']
                log(f"[{case['id']}] No-go zone: x {z['x1']}~{z['x2']}, y {z['y1']}~{z['y2']}", hl=True)
                send(cse, case, 'final', 'floor')
                publish_zones(cse, zones)
                case = None

        if case and now >= case['next_check']:
            case['next_check'] = now + CHECK_S
            for v in case['verifiers']:
                _, tel = cse.la(f'{v}/telemetry')
                vp, d = closest(tel, case['pos']) if isinstance(tel, dict) else (None, None)
                if d is not None and d <= NEAR_M and case['near_since'] is None:
                    case['near_since'] = now
                    case['next_check'] = now + GRACE_S
                    log(f"[{case['id']}] {v} passed the suspect location ({d:.1f} m) -> waiting {GRACE_S:.0f} s for its report")
                    break
        if case and case['near_since'] and now - case['near_since'] >= GRACE_S:
            log(f"[{case['id']}] No anomaly from cross-check robot -> FINAL: ROBOT anomaly ({case['target']})", hl=True)
            send(cse, case, 'final', 'robot')
            muted[case['target']] = cse.la(f"{case['target']}/command")[0]
            case = None
        if case and now - case['t0'] > CASE_TIMEOUT_S:
            log(f"[{case['id']}] No conclusion within {CASE_TIMEOUT_S:.0f} s -> case closed")
            case = None


def cleared(cse, name, muted):
    ri, cmd = cse.la(f'{name}/command')
    if ri != muted[name] and isinstance(cmd, dict) and cmd.get('command') == 'clear_fault':
        del muted[name]
        log(f'{name} maintenance done (clear_fault) -> monitoring resumed')
        return True
    return False


def merge_zone(p1, p2):
    z = {}
    for k, a, b in (('x', p1[0], p2[0]), ('y', p1[1], p2[1])):
        lo, hi = min(a, b) - ZONE_MARGIN, max(a, b) + ZONE_MARGIN
        if hi - lo < ZONE_MIN:
            c = (lo + hi) / 2
            lo, hi = c - ZONE_MIN / 2, c + ZONE_MIN / 2
        z[k + '1'], z[k + '2'] = round(lo, 2), round(hi, 2)
    return z


def inside(p, z):
    return z['x1'] <= p[0] <= z['x2'] and z['y1'] <= p[1] <= z['y2']


def publish_zones(cse, zones):
    cse.cin(f'{AE_NAME}/{CNT}', {'zones': [{k: z[k] for k in ('x1', 'y1', 'x2', 'y2')} for z in zones]})


def send(cse, case, stage, cause=None):
    con = {'type': 'verdict', 'case_id': case['id'], 'stage': stage, 'site': SITE,
           'target': 'C' + case['target'], 'pos': {'x': case['pos'][0], 'y': case['pos'][1]}}
    if cause:
        con['cause'] = cause
    if cause == 'floor':
        con['zone'] = case['zone']
    ok = [r for r in ROBOTS if cse.cin(f'{r}/command', con)]
    if stage == 'final':
        log(f"[{case['id']}] Verdict sent to {', '.join(ok) or 'no robot'}")


def main():
    global CHECK_S, GRACE_S, CASE_TIMEOUT_S, ZONE_MARGIN, ZONE_MIN, ALIVE_S
    ap = argparse.ArgumentParser()
    ap.add_argument('--site', default='C', type=lambda v: v.upper()[4:] if v.upper().startswith('SITE') else v.upper(), choices=sorted(SITES),
                    help='site to judge: A, B or C (default C)')
    ap.add_argument('--robots', help='comma-separated robot AE names (default: all robots of the site)')
    ap.add_argument('--cse', required=True, help='CSE base URL incl. CSE name, e.g. http://<host>:<port>/<CSE name>')
    ap.add_argument('--notify', help='URL where the CSE sends notifications, e.g. http://10.1.2.3:9090. '
                                     'Omit to receive via pollingChannel (when the CSE cannot reach this PC)')
    ap.add_argument('--port', type=int, default=9090)
    ap.add_argument('--ca', help='CA certificate file for an https CSE with a self-signed certificate')
    ap.add_argument('--insecure', action='store_true', help='do not verify the https CSE certificate')
    ap.add_argument('--check-s', type=float, help=f'cross-check position poll interval s (default {CHECK_S:.0f})')
    ap.add_argument('--grace-s', type=float, help=f'wait after the cross-check robot passes, s (default {GRACE_S:.0f})')
    ap.add_argument('--zone-margin', type=float, help=f'no-go zone margin around both locations, m (default {ZONE_MARGIN:g})')
    ap.add_argument('--zone-min', type=float, help=f'no-go zone minimum width/height, m (default {ZONE_MIN:g})')
    ap.add_argument('--timeout-s', type=float, help=f'max case duration s (default {CASE_TIMEOUT_S:.0f})')
    ap.add_argument('--alive-s', type=float, help=f'CSE liveness check interval s (default {ALIVE_S:g})')
    ap.add_argument('--iot', help='MEC 033 registered_iot_platforms URL of the MEP. While the CSE is down, '
                                  'ask it for the current CSE and move there if it changed')
    a = ap.parse_args()
    configure(a.site, [r.strip() for r in a.robots.split(',') if r.strip()] if a.robots else None)
    ALIVE_S = a.alive_s or ALIVE_S
    CHECK_S = a.check_s or CHECK_S
    GRACE_S = a.grace_s or GRACE_S
    CASE_TIMEOUT_S = a.timeout_s or CASE_TIMEOUT_S
    ZONE_MARGIN = a.zone_margin if a.zone_margin is not None else ZONE_MARGIN
    ZONE_MIN = a.zone_min if a.zone_min is not None else ZONE_MIN
    def stop(*_):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, stop)
    cse = CSE(a.cse, verify=False if a.insecure else (a.ca or True))
    srv = None
    if a.notify:
        srv = ThreadingHTTPServer(('0.0.0.0', a.port), Handler)
        threading.Thread(target=srv.serve_forever, daemon=True).start()
    setup_ae(cse, a.notify)
    if not a.notify:
        threading.Thread(target=pch_loop, args=(cse,), daemon=True).start()
        if not cse.ready.wait(15):
            raise RuntimeError('pollingChannel not ready')
    setup(cse, a.notify)
    threading.Thread(target=watch, args=(cse, a.notify, a.iot), daemon=True).start()
    log(f'{AE_NAME} ready: monitoring {SITE} ({", ".join(ROBOTS)}), CSE check every {ALIVE_S:g} s'
        + (', CSE move via MEC 033' if a.iot else ''))
    try:
        judge(cse)
    except KeyboardInterrupt:
        pass
    finally:
        for p in (f'{SOURCE}/{SUB_RN}', f'{REQUEST}/{SUB_REQ_RN}'):
            try:
                cse.req('DELETE', p, timeout=3)
            except requests.RequestException:
                pass
        log(f'{AE_NAME} stopped (subscriptions removed)')
        if srv:
            srv.shutdown()


if __name__ == '__main__':
    main()
