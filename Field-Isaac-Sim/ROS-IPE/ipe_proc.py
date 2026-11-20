# oneM2M IPE for one robot (one process per ROS_DOMAIN_ID).
#   - Samples IMU / joint / TF topics into 1 s windows -> <AE>/telemetry CIN; demo state -> <AE>/status CIN.
#   - Commands written to <AE>/command (subscription + pollingChannel, fallback: polling command/la)
#     are forwarded as JSON to <topic_prefix>cmd for the task executor / demo script (-> Nav2).
#   - Uplink is buffered: on CSE outage data is kept and sent in order after reconnecting; then one outage
#     record {robot, site, disconnect_duration_ms, buffered_count, data_loss} is posted to <AE>/log.
#   - Optional CSE move (fleet.yaml sites.<site>.iot): while disconnected, the MEP's MEC 033 API is asked for the
#     current CSE; if it changed, the IPE re-registers there and sends the backlog.
# Run (normally): ros_ipe/run_ipe.sh [domains...]
# Direct:  ROS_DOMAIN_ID=<n> python -u ipe_proc.py --ros-args --params-file ipe_common.yaml -p fleet_file:=fleet.yaml
# Config:  ipe_common.yaml (joint names, IMU mounts, retention), fleet.yaml (domain -> site/robot/CSE/ACP;
#          see fleet.example.yaml).
import collections, json, math, os, queue, re, ssl, threading, time, urllib.parse, uuid
import yaml
import requests, rclpy, tf2_ros
from rclpy.node import Node
from rclpy.clock import Clock, ClockType
from rclpy.qos import qos_profile_sensor_data as SENSOR_QOS
from rclpy.time import Time
from sensor_msgs.msg import Imu, JointState
from std_msgs.msg import Int64, String

IMUS = ['front', 'left', 'right']                      # topics imu_<name>; the first one clocks the samples
IMU_KEYS = ['ax', 'ay', 'az', 'gx', 'gy', 'gz', 'roll', 'pitch']
ENC = {'drive': ['pos', 'vel', 'eff'], 'swivel': ['pos'],
       'roller_fl': ['pos', 'vel'], 'roller_fr': ['pos', 'vel'],
       'roller_rl': ['pos', 'vel'], 'roller_rr': ['pos', 'vel'],
       'lift': ['pos', 'eff']}                         # lift eff (N): payload kg ~ eff/9.81 - lift base mass (at rest)
FIELD = {'pos': 0, 'vel': 1, 'eff': 2}
CMN_AE = {'siteA': 'CMN-AE-1', 'siteB': 'CMN-AE-2', 'siteC': 'CMN-AE-3'}   # default per site; fleet.yaml cmn_ae overrides
TEL_READERS = {f'site{s}': [f'CTRAINING-AE-{i}', f'CANALYSIS-AE-{i}'] for i, s in ((1, 'A'), (2, 'B'), (3, 'C'))}
ACP_RN = 'acp_telemetry'
ACOP_ALL = 63
ACOP_CMN = 1 + 2 + 16 + 32                             # CREATE + RETRIEVE + NOTIFY + DISCOVERY = 51
JUDGE_AE = {'siteA': 'CJUDGE-AE-1', 'siteB': 'CJUDGE-AE-2', 'siteC': 'CJUDGE-AE-3'}
ACP_CMD_RN = 'acp_command'
ACOP_WRITER = 1 + 2 + 32                               # CREATE + RETRIEVE + DISCOVERY = 35
ADMIN = 'CAdmin'
ACOP_JUDGE_TEL = 2 + 32                                # RETRIEVE + DISCOVERY = 34
ACP_LOG_RN = 'acp_log'                                 # log container: only the robot itself and CAdmin
PCH_RN = 'pch'
SUB_RN = 'sub_ipe_cmd'
# Isaac wraps revolute joint positions to [-2pi, 2pi]; these are unwrapped into cumulative angles and
# their vel is recomputed from dpos/dt (Isaac joint velocity spikes when a wheel starts rolling).
WRAP_ROLES = ('drive', 'roller_fl', 'roller_fr', 'roller_rl', 'roller_rr')

def rnd(v, n=4):
    return None if v is None or not math.isfinite(v) else round(v, n)


# Quaternions are (x, y, z, w) tuples.
def rpy(q):
    x, y, z, w = q
    return (math.atan2(2 * (w * x + y * z), 1 - 2 * (x * x + y * y)),
            math.asin(max(-1.0, min(1.0, 2 * (w * y - z * x)))),
            math.atan2(2 * (w * z + x * y), 1 - 2 * (y * y + z * z)))


def qmul(a, b):
    ax, ay, az, aw = a
    bx, by, bz, bw = b
    return (aw * bx + ax * bw + ay * bz - az * by, aw * by - ax * bz + ay * bw + az * bx,
            aw * bz + ax * by - ay * bx + az * bw, aw * bw - ax * bx - ay * by - az * bz)


def qconj(q):
    return (-q[0], -q[1], -q[2], q[3])


def qrot(q, v):
    return qmul(qmul(q, (*v, 0.0)), qconj(q))[:3]


def imu_row(m, q_bi):
    """IMU sample in base_link axes. q_bi = IMU mounting orientation in base_link."""
    if m is None:
        return [None] * 8
    a, g, o = m.linear_acceleration, m.angular_velocity, m.orientation
    acc = qrot(q_bi, (a.x, a.y, a.z))
    gyr = qrot(q_bi, (g.x, g.y, g.z))
    q_wb = qmul((o.x, o.y, o.z, o.w), qconj(q_bi))
    return [*acc, *gyr, *rpy(q_wb)[:2]]


class WsCse:
    """oneM2M WebSocket binding (TS-0020, oneM2M.json); req() returns (HTTP-like status, body) like the HTTP path."""
    OP = {'POST': 1, 'GET': 2, 'PUT': 3, 'DELETE': 4}
    RSC = {2000: 200, 2001: 201, 2002: 200, 2004: 200, 4000: 400, 4004: 404, 4005: 405, 4008: 408, 4015: 415,
           4101: 403, 4103: 403, 4106: 403, 4107: 403, 4117: 403, 4105: 409, 5000: 500, 5204: 504}
    INT_FC = {'fu', 'lim', 'lvl', 'ofst', 'sts', 'stb', 'sza', 'szb'}
    PING_S, DEAD_S = 1.0, 5.0   # ping when idle for PING_S; no frame at all for DEAD_S -> treat as disconnected
    MIN_GAP_S = 0.0             # min gap between sent messages (0: requests are already serialized)
    # ACME's WebSocket server hands each message to a worker through a lambda that captures the variable, not the
    # value, so a message arriving before the previous one is dispatched replaces it. Requests are therefore sent
    # one at a time (req_lock), and the long pcu poll goes over HTTP (IPE.req(..., http=True)).

    def __init__(self, url, origin, cse_rn, verify, log, name, ping_s=None, dead_s=None, min_gap_s=None):
        try:
            import websocket                        # pip install websocket-client
        except ImportError:
            raise SystemExit('binding: ws requires websocket-client: pip install websocket-client')
        self.wsmod, self.url, self.origin, self.cse_rn, self.log, self.name = websocket, url, origin, cse_rn.strip('/'), log, name
        self.sslopt = {} if verify else {'cert_reqs': ssl.CERT_NONE, 'check_hostname': False}
        self.ping_s, self.dead_s = float(ping_s or self.PING_S), float(dead_s or self.DEAD_S)
        self.min_gap = float(self.MIN_GAP_S if min_gap_s is None else min_gap_s)
        self.last_tx = 0.0
        self.conn, self.up = None, threading.Event()
        self.send_lock, self.plock, self.pending = threading.Lock(), threading.Lock(), {}
        self.req_lock = threading.Lock()           # one request awaiting a response at a time
        threading.Thread(target=self.loop, daemon=True).start()

    def loop(self):
        first = True
        while True:
            ws = None
            try:
                ws = self.wsmod.create_connection(self.url, subprotocols=['oneM2M.json'], sslopt=self.sslopt, timeout=10,
                                                  header=[f'X-M2M-Origin: {self.origin}'])   # ACME: without it, everything but AE registration gets 403
                ws.settimeout(self.ping_s)
                self.conn = ws
                self.up.set()
                self.log.info(f'WebSocket connected ({self.name}): {self.url}')
                first = True
                last_rx = time.monotonic()
                while True:
                    try:
                        op, data = ws.recv_data(control_frame=True)
                    except self.wsmod.WebSocketTimeoutException:
                        # a dropped Wi-Fi link raises no TCP error, so silence for dead_s means disconnected
                        if time.monotonic() - last_rx > self.dead_s:
                            raise ConnectionError(f'nothing received for {self.dead_s:g} s (network down)')
                        with self.send_lock:
                            ws.ping()
                        continue
                    last_rx = time.monotonic()
                    if op == self.wsmod.ABNF.OPCODE_CLOSE:
                        raise ConnectionError('closed by CSE')
                    if op in (self.wsmod.ABNF.OPCODE_TEXT, self.wsmod.ABNF.OPCODE_BINARY):
                        self.on_msg(data.decode('utf-8', 'replace') if isinstance(data, bytes) else data)
            except Exception as e:
                if first:
                    self.log.warn(f'WebSocket ({self.name}) disconnected/connect failed: {e} -> retrying every 3 s')
                first = False
            self.up.clear()
            self.conn = None
            if ws:
                try:
                    ws.shutdown()                         # close the dead socket without a close handshake
                except Exception:
                    pass
            with self.plock:
                for ev in self.pending.values():
                    ev[1] = None
                    ev[0].set()
                self.pending.clear()
            time.sleep(3.0)

    def retarget(self, url, cse_rn):
        """CSE moved: reconnect to the new URL (drops the current connection, if any)."""
        self.url, self.cse_rn = url, cse_rn.strip('/')
        ws = self.conn
        if ws:
            try:
                ws.shutdown()
            except Exception:
                pass

    def on_msg(self, msg):
        try:
            m = json.loads(msg)
        except ValueError:
            return
        if 'rsc' in m and 'op' not in m:
            with self.plock:
                ev = self.pending.get(m.get('rqi'))
            if ev:
                ev[1] = m
                ev[0].set()
            else:                                  # response to a request that already timed out (late, not lost)
                self.log.warn(f'WebSocket ({self.name}) late response rqi={m.get("rqi")} rsc={m.get("rsc")}', throttle_duration_sec=5.0)
        elif 'op' in m:                            # CSE-initiated request: just ack (commands arrive via pollingChannel)
            self.send({'rsc': 2000, 'rqi': m.get('rqi'), 'rvi': '3', 'to': m.get('fr'), 'fr': self.origin})

    def send(self, prim):
        with self.send_lock:
            wait = self.last_tx + self.min_gap - time.monotonic()
            if wait > 0:
                time.sleep(wait)
            self.conn.send(json.dumps(prim))
            self.last_tx = time.monotonic()

    def req(self, method, path, ty, body, timeout):
        path, _, q = path.partition('?')
        prim = {'op': self.OP[method], 'to': f'{self.cse_rn}/{path}'.rstrip('/'), 'fr': self.origin,
                'rqi': uuid.uuid4().hex[:12], 'rvi': '3'}
        if ty:
            prim['ty'] = ty
        if body is not None:
            prim['pc'] = body
        fc = {}
        for k, v in urllib.parse.parse_qsl(q):
            if k == 'rcn':
                prim['rcn'] = int(v)
            elif k in ('lbl', 'ty'):               # must be lists (ACME: 'expected type: list')
                fc.setdefault(k, []).append(int(v) if k == 'ty' else v)
            else:
                fc[k] = int(v) if k in self.INT_FC and v.lstrip('-').isdigit() else v
        if fc:
            prim['fc'] = fc
        t_end = time.monotonic() + timeout
        if not self.up.wait(timeout):
            raise requests.ConnectionError(f'WebSocket ({self.name}) not connected')
        if not self.req_lock.acquire(timeout=max(0.0, t_end - time.monotonic())):
            raise requests.Timeout(f'WebSocket ({self.name}) previous request still pending after {timeout} s')
        ev = [threading.Event(), None]
        with self.plock:
            self.pending[prim['rqi']] = ev
        try:
            self.send(prim)
            if not ev[0].wait(timeout):
                raise requests.Timeout(f'WebSocket ({self.name}) no response within {timeout} s')
        except requests.Timeout:
            raise
        except Exception as e:
            raise requests.ConnectionError(f'WebSocket ({self.name}) send failed: {e}')
        finally:
            with self.plock:
                self.pending.pop(prim['rqi'], None)
            self.req_lock.release()
        m = ev[1]
        if m is None:
            raise requests.ConnectionError(f'WebSocket ({self.name}) disconnected before response')
        rsc = int(m.get('rsc', 5000))
        code = self.RSC.get(rsc) or (200 if rsc < 3000 else 400 if rsc < 5000 else 500)
        return code, m.get('pc') or {}


class IPE(Node):
    def __init__(self):
        super().__init__('onem2m_ipe')
        P = lambda k, d: self.declare_parameter(k, d).value
        self.domain = int(os.environ.get('ROS_DOMAIN_ID', '0'))
        fleet_file = os.path.expanduser(P('fleet_file', 'fleet.yaml'))
        fleet = yaml.safe_load(open(fleet_file))
        robots = {int(k): v for k, v in (fleet.get('robots') or {}).items()}
        if self.domain not in robots:
            raise SystemExit(f'{fleet_file}: no robot with ROS_DOMAIN_ID={self.domain} '
                             f'(configured domains: {sorted(robots)})')
        me = robots[self.domain]
        self.site, self.robot = me['site'], me['name']
        site_cfg = (fleet.get('sites') or {}).get(self.site)
        if site_cfg is None:
            raise SystemExit(f'{fleet_file}: no entry for {self.site} under sites')
        self.ae = f"{site_cfg.get('ae_prefix', self.site)}_{self.robot}"   # e.g. siteA_AMR_1
        self.origin = 'C' + self.ae
        self.url = site_cfg['cse_url'].rstrip('/')
        self.cmn_ae = site_cfg.get('cmn_ae') or next(
            (v for k, v in CMN_AE.items() if self.ae.startswith(k + '_')), None)
        judge = site_cfg.get('judge_ae') or next((v for k, v in JUDGE_AE.items() if self.ae.startswith(k + '_')), None)
        self.judge_ae = judge
        self.tel_readers = site_cfg.get('tel_readers') or next(
            (v for k, v in TEL_READERS.items() if self.ae.startswith(k + '_')), [])
        self.cmd_writers = [w for w in [judge, *(site_cfg.get('cmd_writers') or [])] if w]
        self.topic_prefix = fleet.get('topic_prefix', 'AMR/')
        self.window_s = P('window_s', 1.0)
        self.jmap = {r: P('joint_' + r, '') for r in ENC}   # role -> joint name
        self.wrap_names = {self.jmap[r] for r in WRAP_ROLES if self.jmap.get(r)}
        self.unwrap, self.last_js_t = {}, None              # joint -> [last raw pos, offset]
        self.vprev = {}                                     # joint -> (t, cumulative pos, vel)
        self.imu_q = {n: tuple(P('imu_quat_' + n, [0.0, 0.0, 0.0, 1.0])) for n in IMUS}

        # mni: CSE drops the oldest CINs beyond this
        self.cnts = {'telemetry': {'mni': P('telemetry_mni', 600)},
                     'status': {'mni': P('status_mni', 600)},
                     'command': {'mni': 20},
                     'log': {'mni': P('log_mni', 200)}}       # outage records

        self.lock = threading.Lock()
        self.imu = dict.fromkeys(IMUS)
        self.joints, self.pose, self.win = {}, (None, None, None), None
        self.seq = {'telemetry': 0, 'status': 0}
        self.state, self.cmd_id, self.demo = 'idle', None, {}
        self.cmds = queue.Queue(50)
        self.seen_ri = collections.deque(maxlen=200)        # dedup commands arriving via both paths
        self.sub_ok = self.pcu_ok = False                   # both true -> fallback polling pauses
        self.res_ready = threading.Event()                  # set once the AE and containers exist (pch/subscription wait for it)
        self.pch_wait_s = P('pch_timeout_s', 70.0)          # max wait per pcu long-poll
        buffer_s = P('buffer_s', 1800.0)                    # seconds of data kept during an outage
        self.buf = collections.deque(maxlen=int(buffer_s * (1.0 / self.window_s + 0.5)))   # telemetry 1/window_s + status 0.5 per s
        self.buf_cv = threading.Condition()
        self.buf_dropped = 0
        self.outage, self.outage_total = None, {'count': 0, 'down_s': 0.0, 'held': 0, 'delivered': 0,
                                                'dropped_overflow': 0, 'dropped_error': 0}
        self.outage_log = collections.deque(maxlen=int(P('outage_keep', 200)))
        m = re.search(r'/(mep-[a-z0-9]+)/', self.url + '/')
        self.mep_site = site_cfg.get('mep_site') or (m.group(1) if m else 'mep-' + self.site[-1].lower())
        self.still_skip_s = P('still_skip_s', 10.0)         # skip telemetry after standing still this long
        self.still_vel = P('still_vel_eps', 0.05)           # drive wheel |vel| (rad/s) threshold
        self.still_gyro = P('still_gyro_eps', 0.05)         # |gz| (rad/s) threshold; 0.02 is below IMU noise at rest (~0.022)
        self.still_since, self.still_skipped = None, 0
        # Local JSONL copy of uploaded CINs (same shape as CSE telemetry.json); dump_dir '' disables
        self.dump_cnts = set(P('dump_cnts', ['telemetry']))
        dump_dir = os.path.expanduser(P('dump_dir', '~/telemetry_dump'))
        self.dump_f, self.dump_n = None, 0
        if dump_dir:
            os.makedirs(dump_dir, exist_ok=True)
            self.dump_path = os.path.join(dump_dir, f"{self.ae}_{time.strftime('%Y%m%d_%H%M%S')}.jsonl")
            self.dump_f = open(self.dump_path, 'a', buffering=1, encoding='utf-8')
        self.http = requests.Session()
        self.http.verify = bool(site_cfg.get('tls_verify', True))   # false for self-signed HTTPS
        if not self.http.verify:
            requests.packages.urllib3.disable_warnings()
        # binding: http (default) | ws. Single WS connection: ACME closes the first when the same originator reconnects.
        self.binding = site_cfg.get('binding', 'http')
        self.ws = None
        if self.binding == 'ws':
            wcfg = site_cfg.get('ws') or {}
            if not wcfg.get('url'):
                raise SystemExit(f'{fleet_file}: sites.{self.site}.ws.url is missing (binding: ws)')
            cse_rn = wcfg.get('cse_rn') or self.url.rsplit('/', 1)[-1]
            self.ws = WsCse(wcfg['url'], self.origin, cse_rn, self.http.verify, self.get_logger(), 'ws',
                            ping_s=wcfg.get('ping_s'), dead_s=wcfg.get('dead_s'), min_gap_s=wcfg.get('min_gap_s'))
        elif self.binding != 'http':
            raise SystemExit(f'{fleet_file}: sites.{self.site}.binding must be http or ws ({self.binding})')
        # CSE move via MEC 033 (sites.<site>.iot): while disconnected, ask the MEP for the current CSE.
        #   MEP unreachable -> network (AP) down, wait / same CSE -> keep retrying / other CSE -> move and re-register
        icfg = site_cfg.get('iot') or {}
        self.iot_url = icfg.get('url')
        self.iot_check_s = float(icfg.get('check_s', 5.0))
        self.iot_keep_host = bool(icfg.get('keep_host', True))
        self.cse_gen = 0                                    # +1 on every move (pch_loop / poll_commands resync)
        self.cse_moved = threading.Event()                  # uplink re-creates resources before sending

        self.tf = tf2_ros.Buffer()
        self.tf_listener = tf2_ros.TransformListener(self.tf, self)
        for n in IMUS:
            self.create_subscription(Imu, f'{self.topic_prefix}imu_{n}', lambda m, n=n: self.on_imu(n, m), SENSOR_QOS)
        self.create_subscription(JointState, f'{self.topic_prefix}joints', self.on_joints, SENSOR_QOS)
        self.cmd_pub = self.create_publisher(String, f'{self.topic_prefix}cmd', 10)
        self.seq_pub = self.create_publisher(Int64, f'{self.topic_prefix}telemetry_seq', 10)
        self.create_subscription(String, f'{self.topic_prefix}demo_state', self.on_demo_state, 10)

        steady = Clock(clock_type=ClockType.STEADY_TIME)        # keeps running while the sim is paused
        self.create_timer(0.1, self.run_commands, clock=steady)
        self.push_status()
        threading.Thread(target=self.uplink, daemon=True).start()
        threading.Thread(target=self.poll_commands, daemon=True).start()
        threading.Thread(target=self.pch_loop, daemon=True).start()
        if self.iot_url:
            threading.Thread(target=self.cse_watch, daemon=True).start()
            self.get_logger().info(f'CSE move enabled: when disconnected, asking the MEP every {self.iot_check_s:g} s ({self.iot_url})')
        self.get_logger().info(f'ROS_DOMAIN_ID={self.domain} -> AE={self.ae}  CSE={self.url} ({self.binding})  topics={self.topic_prefix}imu_*, '
                               f'{self.topic_prefix}joints, {self.topic_prefix}cmd(pub), {self.topic_prefix}demo_state(sub)')
        if self.dump_f:
            self.get_logger().info(f'Saving telemetry to file: {self.dump_path}')
        missing = [r for r, j in self.jmap.items() if not j]
        if missing:
            self.get_logger().warn(f'Joint name parameters missing -> values will be null: {", ".join("joint_" + r for r in missing)}')

    # ---------- oneM2M ----------
    def req(self, method, path='', ty=None, body=None, timeout=3, http=False):
        """One CSE request -> (HTTP status, body). http=True forces HTTP even with binding: ws."""
        if self.ws and not http:
            return self.ws.req(method, path, ty, body, timeout)
        h = {'X-M2M-Origin': self.origin, 'X-M2M-RI': uuid.uuid4().hex[:12],
             'X-M2M-RVI': '3', 'Accept': 'application/json'}
        if ty:
            h['Content-Type'] = f'application/json;ty={ty}'
        elif body is not None:
            h['Content-Type'] = 'application/json'
        r = self.http.request(method, f'{self.url}/{path}'.rstrip('/'), headers=h,
                              data=json.dumps(body) if body else None, timeout=timeout)
        try:
            return r.status_code, r.json()
        except ValueError:
            return r.status_code, {}

    def ensure_resources(self):
        while True:
            try:
                if self.req('GET', self.ae)[0] == 200:
                    self.get_logger().info(f'AE {self.ae} already exists -> reusing')
                else:
                    c, _ = self.req('POST', '', 2, {'m2m:ae': {
                        'rn': self.ae, 'api': 'Nedgeway.forklift.ipe', 'rr': False, 'srv': ['3'],
                        'lbl': [f'site:{self.site}', f'robot:{self.robot}', f'domain:{self.domain}']}})
                    if c not in (201, 403, 409):      # 403/409 = already registered
                        raise RuntimeError(f'AE create {c}')
                acpi = {'telemetry': [self.ensure_acp(ACP_RN, [([self.cmn_ae, *self.tel_readers], ACOP_CMN),
                                                               ([self.judge_ae], ACOP_JUDGE_TEL)])]}
                if self.cmd_writers:
                    acpi['command'] = [self.ensure_acp(ACP_CMD_RN, [(self.cmd_writers, ACOP_WRITER)])]
                acpi['log'] = [self.ensure_acp(ACP_LOG_RN, [])]
                for rn, attrs in self.cnts.items():
                    want = acpi.get(rn)
                    lbl = self.cnt_labels(rn)
                    c, js = self.req('GET', f'{self.ae}/{rn}')
                    if c == 200:
                        cur = js.get('m2m:cnt', {})
                        upd = {**({'acpi': want} if want and cur.get('acpi') != want else {}),
                               **({'lbl': lbl} if sorted(cur.get('lbl') or []) != sorted(lbl) else {})}
                        if upd:
                            c, _ = self.req('PUT', f'{self.ae}/{rn}', body={'m2m:cnt': upd})
                            if c not in (200, 204):
                                raise RuntimeError(f'{rn} acpi/lbl update {c}')
                            self.get_logger().info(f'{rn} container updated: {upd}')
                        continue
                    body = {'rn': rn, **attrs, 'lbl': lbl, **({'acpi': want} if want else {})}
                    c, _ = self.req('POST', self.ae, 3, {'m2m:cnt': body})
                    if c not in (201, 409):
                        raise RuntimeError(f'{rn} create {c}')
                self.get_logger().info('oneM2M resources ready')
                self.res_ready.set()
                return
            except Exception as e:
                self.get_logger().warn(f'CSE setup failed, retrying in 5 s (buffered {len(self.buf)}): {e}')
                time.sleep(5)

    def cnt_labels(self, rn):
        # ACME ORs multiple lbl filters, so site+type is also a single label, e.g. lbl=siteC:telemetry
        return [f'site:{self.site}', f'robot:{self.robot}', f'ae:{self.ae}', f'type:{rn}', f'{self.site}:{rn}']

    def ensure_acp(self, rn, rules):
        """Create or fix ACP <AE>/<rn> and return its ri. rules = [(originators, acop), ...]; nobody else has access."""
        acr = [{'acor': [self.origin, ADMIN], 'acop': ACOP_ALL}]
        for who, acop in rules:
            who = [o for o in who if o]
            if who:
                acr.append({'acor': who, 'acop': acop})
        others = [(r['acor'], r['acop']) for r in acr[1:]]
        pv = {'acr': acr}
        pvs = {'acr': [{'acor': [self.origin, ADMIN], 'acop': ACOP_ALL}]}
        path = f'{self.ae}/{rn}'
        c, js = self.req('GET', path)
        if c == 200:
            cur = js.get('m2m:acp', {})
            if cur.get('pv') != pv or cur.get('pvs') != pvs:
                c2, _ = self.req('PUT', path, body={'m2m:acp': {'pv': pv, 'pvs': pvs}})
                if c2 not in (200, 204):
                    raise RuntimeError(f'ACP {rn} update {c2}')
                self.get_logger().info(f'ACP {rn} permissions updated: {self.origin}/{ADMIN}={ACOP_ALL}, {others}')
            ri = cur.get('ri')
        else:
            c, js = self.req('POST', self.ae, 1, {'m2m:acp': {'rn': rn, 'pv': pv, 'pvs': pvs}})
            if c not in (201, 409):
                raise RuntimeError(f'ACP {rn} create {c}')
            ri = js.get('m2m:acp', {}).get('ri')
            self.get_logger().info(f'ACP {rn} created: {self.origin}/{ADMIN}={ACOP_ALL}, {others}')
        if not ri:
            c, js = self.req('GET', path)
            ri = js.get('m2m:acp', {}).get('ri') if c == 200 else None
        if not ri:
            raise RuntimeError(f'ACP {rn}: could not get resource ID')
        return ri

    # ---------- local file ----------
    def dump(self, cnt, data, cin=None):
        if not self.dump_f or cnt not in self.dump_cnts:
            return
        cin = cin or {}
        try:
            self.dump_f.write(json.dumps({'ri': cin.get('ri'), 'rn': cin.get('rn'), 'ct': cin.get('ct'), 'lbl': None,
                                          'con': data}, allow_nan=False, separators=(',', ':')) + '\n')
            self.dump_n += 1
        except Exception as e:
            self.get_logger().warn(f'Telemetry file write failed: {e}', throttle_duration_sec=10.0)

    def close_dump(self):
        if self.outage:
            self.outage_close(partial=True)
        if self.dump_f:
            self.dump_f.close()
            print(f'[ipe] shutdown: telemetry file {self.dump_path} ({self.dump_n} lines)', flush=True)
            self.dump_f = None

    def uplink(self):
        """Send the buffer head in order; it is removed only after a final result."""
        self.ensure_resources()
        backlog_warned = False
        while True:
            with self.buf_cv:
                while not self.buf:
                    self.buf_cv.wait()
                item = self.buf[0]
            cnt, data = item
            if self.cse_moved.is_set():               # moved to another CSE -> create resources there first
                self.cse_moved.clear()
                self.ensure_resources()
            try:
                con = json.dumps(data, allow_nan=False, separators=(',', ':'))
                c, js = self.req('POST', f'{self.ae}/{cnt}', 4,
                                 {'m2m:cin': {'cnf': 'application/json:0', 'con': con}})
            except Exception as e:
                self.outage_fail(f'not connected: {e}')
                self.get_logger().warn(f'CSE unreachable -> {len(self.buf)} items buffered (retrying in 2 s): {e}',
                                       throttle_duration_sec=10.0)
                backlog_warned = True
                time.sleep(2.0)
                continue
            if c in (403, 404) or c >= 500:
                self.outage_fail(f'CSE response {c}')
            if c == 404:                              # CSE restarted and lost the AE/containers
                self.get_logger().warn(f'{cnt} 404 -> re-creating oneM2M resources')
                self.ensure_resources()
                continue
            if c == 403:
                self.get_logger().warn(f'{cnt} CIN 403 (access denied) -> keeping {len(self.buf)} buffered, rechecking ACP in 5 s',
                                       throttle_duration_sec=10.0)
                time.sleep(5.0)
                self.ensure_resources()
                continue
            if c >= 500:
                self.get_logger().warn(f'{cnt} CIN failed {c} -> keeping {len(self.buf)} buffered, retrying in 2 s',
                                       throttle_duration_sec=10.0)
                time.sleep(2.0)
                continue
            if c != 201:
                self.get_logger().warn(f'{cnt} CIN failed {c} -> dropping this item', throttle_duration_sec=5.0)
            with self.buf_cv:
                if self.buf and self.buf[0] is item:  # may have been pushed out by overflow meanwhile
                    self.buf.popleft()
                    self.dump(cnt, data, (js or {}).get('m2m:cin') if c == 201 else None)
                    self.outage_sent(c == 201)
                left = len(self.buf)
            if cnt == 'log':
                self.get_logger().info(f'log CIN {"posted" if c == 201 else f"failed {c}"}: {con}')
            if backlog_warned and left == 0:
                self.get_logger().info('CSE reconnected -> backlog sent')
                backlog_warned = False

    def send(self, cnt, data, stamp=True):
        if stamp:                                     # log records are posted as-is (no v/seq)
            self.seq[cnt] += 1
            data.update(v=2, robot=self.ae, seq=self.seq[cnt])
        with self.buf_cv:
            if len(self.buf) == self.buf.maxlen:
                self.buf_dropped += 1
                if self.outage and self.outage['_left'] > 0:
                    self.outage['dropped_overflow'] += 1
                    self.outage['_left'] -= 1
                self.get_logger().warn(f'Buffer full ({self.buf.maxlen}) -> dropping oldest data '
                                       f'(total dropped {self.buf_dropped})', throttle_duration_sec=10.0)
                self.dump(*self.buf[0])
            self.buf.append((cnt, data))
            if self.outage and self.outage['recovered'] is None:
                self.outage['held'] += 1
                self.outage['_left'] += 1
                self.outage['max_backlog'] = max(self.outage['max_backlog'], len(self.buf))
            self.buf_cv.notify()

    # ---------- outage tracking (in memory only) ----------
    def outage_fail(self, why):
        with self.buf_cv:
            o = self.outage
            if o is None or o['recovered'] is not None:
                if o is not None:                     # dropped again while draining -> same outage
                    o['recovered'] = None
                    return
                n = len(self.buf)
                self.outage = {'start': time.time(), 'recovered': None, 'drained': None, 'why': why,
                               'held': n, 'delivered': 0, 'dropped_overflow': 0, 'dropped_error': 0,
                               'max_backlog': len(self.buf), '_left': n}
                self.get_logger().warn(f'Outage started ({why}) -> holding from {n} buffered')

    def outage_sent(self, ok):
        o = self.outage
        if o is None:
            return
        if o['recovered'] is None and ok:
            o['recovered'] = time.time()
            self.get_logger().info(f'CSE reconnected (down {o["recovered"] - o["start"]:.1f} s) -> sending {o["_left"]} backlogged')
        if o['_left'] > 0:
            o['delivered' if ok else 'dropped_error'] += 1
            o['_left'] -= 1
        if o['recovered'] is not None and o['_left'] == 0:
            self.outage_close()

    def outage_close(self, partial=False):
        """Close one outage record. event: BUFFER_FLUSHED (backlog sent) | BUFFER_ABANDONED (IPE exited first)."""
        o, self.outage = self.outage, None
        if o is None:
            return
        now = time.time()
        down_s = (o['recovered'] or now) - o['start']
        loss = o['dropped_overflow'] + o['dropped_error'] + (o['_left'] if partial else 0)
        ev = {'source': 'ROBOT-IPE', 'site': self.mep_site, 'robot': self.ae,
              'event': 'BUFFER_ABANDONED' if partial else 'BUFFER_FLUSHED',
              'disconnect_duration_ms': int(round(down_s * 1000)), 'buffered_count': o['held'], 'data_loss': loss,
              'timestamp': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime(now))}
        t = self.outage_total
        t['count'] += 1
        t['down_s'] = round(t['down_s'] + down_s, 3)
        t['held'] += o['held']
        t['delivered'] += o['delivered']
        t['dropped_overflow'] += o['dropped_overflow']
        t['dropped_error'] += o['dropped_error'] + (o['_left'] if partial else 0)
        self.get_logger().info(f'Outage record: {down_s:.1f} s, held {o["held"]} -> delivered {o["delivered"]} / lost {loss} '
                               f'(overflow {o["dropped_overflow"]}, error {o["dropped_error"]}{", unsent at exit " + str(o["_left"]) if partial else ""})')
        self.outage_log.append(ev)
        if not partial:                               # backlog sent -> post the record to <AE>/log (not at exit: no link)
            self.send('log', {'robot': self.ae, 'site': self.mep_site,
                              'disconnect_duration_ms': ev['disconnect_duration_ms'],
                              'buffered_count': o['held'], 'data_loss': loss}, stamp=False)

    # ---------- CSE move (MEC 033) ----------
    def mep_cse(self):
        """MEP registered_iot_platforms -> (CSE-ID, cse_url, ws_url); raises if the MEP is unreachable.
        Uses customServicesTransportInfo entries whose id is a CSE-ID: name = CSE name, endpoint.uris[0] = CSE base."""
        r = self.http.get(self.iot_url, headers={'Accept': 'application/json'}, timeout=3)
        r.raise_for_status()
        q = urllib.parse.urlsplit(self.iot_url)
        found = []
        for p in r.json() or []:
            if p.get('enabled') is False:
                continue
            for ti in p.get('customServicesTransportInfo') or []:
                tid, name = str(ti.get('id') or ''), ti.get('name')
                uris = (ti.get('endpoint') or {}).get('uris') or []
                if not tid.startswith('/') or not name or not uris:
                    continue
                u = urllib.parse.urlsplit(uris[0].rstrip('/'))
                if self.iot_keep_host:                # the MEP reports an internal host -> use the iot.url host
                    u = u._replace(scheme=q.scheme, netloc=q.netloc)
                ws = urllib.parse.urlunsplit(u._replace(scheme='wss' if u.scheme == 'https' else 'ws')) + '/ws'
                found.append((tid, f'{urllib.parse.urlunsplit(u)}/{name}', ws))
        if not found:
            raise RuntimeError('no CSE in the response (customServicesTransportInfo, id=/<CSE-ID>)')
        return next((f for f in found if f[1] == self.url), found[0])

    def cse_watch(self):
        """While an outage is open (not yet recovered), ask the MEP every check_s."""
        while True:
            time.sleep(self.iot_check_s)
            o = self.outage
            if o is None or o['recovered'] is not None:
                continue
            try:
                tid, url, ws_url = self.mep_cse()
            except Exception as e:
                self.get_logger().warn(f'MEP not responding -> network (AP) down, waiting: {e}', throttle_duration_sec=30.0)
                continue
            if url == self.url:
                self.get_logger().info(f'MEP: CSE unchanged ({tid}) -> retrying the same CSE', throttle_duration_sec=30.0)
                continue
            self.switch_cse(tid, url, ws_url)

    def switch_cse(self, tid, url, ws_url):
        """Resources are re-created by uplink (cse_moved), pch/subscription by pch_loop and polling by poll_commands (cse_gen)."""
        old, self.url = self.url, url
        m = re.search(r'/(mep-[a-z0-9]+)/', url + '/')
        if m:
            self.mep_site = m.group(1)
        self.sub_ok = self.pcu_ok = False
        self.res_ready.clear()
        self.cse_gen += 1
        if self.ws:
            self.ws.retarget(ws_url, url.rsplit('/', 1)[-1])
        self.cse_moved.set()
        self.get_logger().warn(f'MEP: CSE moved {old} -> {url} ({tid}) -> re-registering there and sending '
                               f'{len(self.buf)} buffered')

    def outage_events(self):
        return list(self.outage_log)

    # ---------- sensors -> telemetry ----------
    def on_joints(self, m):
        t = m.header.stamp.sec + m.header.stamp.nanosec * 1e-9
        with self.lock:
            if self.last_js_t is not None and t < self.last_js_t - 0.5:   # sim time reset (Stop -> Play)
                self.unwrap.clear()
                self.vprev.clear()
            self.last_js_t = t
            for i, n in enumerate(m.name):
                pos, vel, eff = (arr[i] if i < len(arr) else None
                                 for arr in (m.position, m.velocity, m.effort))
                if n in self.wrap_names and pos is not None and math.isfinite(pos):
                    st = self.unwrap.setdefault(n, [pos, 0.0])
                    d = pos - st[0]
                    if d > 2 * math.pi:
                        st[1] -= 4 * math.pi
                    elif d < -2 * math.pi:
                        st[1] += 4 * math.pi
                    st[0] = pos
                    pos += st[1]
                    pt = self.vprev.get(n)
                    if pt is None:
                        vel = None
                    elif t > pt[0]:
                        vel = (pos - pt[1]) / (t - pt[0])
                    else:
                        vel = pt[2]
                    if pt is None or t > pt[0]:
                        self.vprev[n] = (t, pos, vel)
                self.joints[n] = (pos, vel, eff)

    def on_imu(self, name, m):
        with self.lock:
            self.imu[name] = m
        if name != IMUS[0]:
            return
        t = m.header.stamp.sec + m.header.stamp.nanosec * 1e-9
        try:
            tr = self.tf.lookup_transform('map', 'base_link', Time()).transform
            q = tr.rotation
            self.pose = (tr.translation.x, tr.translation.y, rpy((q.x, q.y, q.z, q.w))[2])
        except tf2_ros.TransformException:
            pass

        with self.lock:
            w = self.win
            if w and (t < w['t_sim'][-1] - 0.5):      # sim time reset -> discard window
                w = None
            if w and t >= w['t0'] + self.window_s:
                if not self.long_still(w):
                    self.send('telemetry', {'t_wall': rnd(w.pop('t_wall'), 3),
                                            'nav': self.state, **{k: w[k] for k in ('t_sim', 'pose', 'imu', 'enc')}})
                    self.seq_pub.publish(Int64(data=self.seq['telemetry']))
                w = None
            if not w:
                w = self.win = {'t0': t, 't_wall': time.time(), 't_sim': [],
                                'pose': {'x': [], 'y': [], 'yaw': []},
                                'imu': {n: {k: [] for k in IMU_KEYS} for n in IMUS},
                                'enc': {r: {f: [] for f in fs} for r, fs in ENC.items()}}
            w['t_sim'].append(rnd(t, 3))
            for k, v in zip(('x', 'y', 'yaw'), self.pose):
                w['pose'][k].append(rnd(v, 3))
            for n in IMUS:
                for k, v in zip(IMU_KEYS, imu_row(self.imu[n], self.imu_q[n])):
                    w['imu'][n][k].append(rnd(v, 5))
            for r, fs in ENC.items():
                vals = self.joints.get(self.jmap[r], (None, None, None))
                for f in fs:
                    w['enc'][r][f].append(rnd(vals[FIELD[f]]))

    def long_still(self, w):
        """True if the robot has been still for more than still_skip_s (telemetry is skipped)."""
        vel = [abs(v) for v in w['enc']['drive']['vel'] if v is not None]
        gz = [abs(v) for v in w['imu'][IMUS[0]]['gz'] if v is not None]
        still = bool(vel) and bool(gz) and max(vel) < self.still_vel and max(gz) < self.still_gyro
        now = time.time()
        if not still:
            if self.still_skipped:
                self.get_logger().info(f'Moving again -> telemetry resumed (skipped {self.still_skipped} while still)')
            self.still_since, self.still_skipped = None, 0
            return False
        if self.still_since is None:
            self.still_since = now
        if now - self.still_since < self.still_skip_s:
            return False
        if self.still_skipped == 0:
            self.get_logger().info(f'{self.still_skip_s:.0f}+ s without moving -> skipping telemetry')
        self.still_skipped += 1
        return True

    # ---------- status ----------
    def push_status(self):
        x, y, yaw = self.pose
        self.send('status', {
            't_wall': rnd(time.time(), 3), 't_sim': rnd(self.get_clock().now().nanoseconds * 1e-9, 3),
            'state': self.state, 'cmd_id': self.cmd_id,
            'pose': {'x': rnd(x, 3), 'y': rnd(y, 3), 'yaw': rnd(yaw)},
            'demo': self.demo})

    def on_demo_state(self, m):
        """JSON from the demo script: 'state' required, 'cmd_id' optional, the rest goes to status.demo."""
        try:
            d = json.loads(m.data)
            s = str(d.pop('state'))
        except (ValueError, KeyError, AttributeError):
            self.get_logger().error(f'Invalid demo_state: {m.data}')
            return
        self.cmd_id = d.pop('cmd_id', self.cmd_id)
        self.demo = d
        if s != self.state:
            self.state = s
            self.get_logger().info(f'state -> {s} (cmd {self.cmd_id})')
            self.push_status()

    # ---------- oneM2M command -> demo script ----------
    def accept_cin(self, cin, via):
        ri = cin.get('ri')
        if ri and ri in self.seen_ri:
            return
        if ri:
            self.seen_ri.append(ri)
        con = cin.get('con')
        try:
            self.cmds.put_nowait(json.loads(con) if isinstance(con, str) else con)
            self.get_logger().info(f'Command received ({via}) ri={ri}')
        except (ValueError, queue.Full) as e:
            self.get_logger().error(f'Could not handle command ({via}) ri={ri}: {e}')

    def notify_ok(self):
        return self.sub_ok and self.pcu_ok

    def poll_commands(self):
        """Fallback: poll command/la every 1 s while subscription/pollingChannel is down. Pre-existing commands are ignored."""
        last, synced, gen = None, False, self.cse_gen
        while True:
            if gen != self.cse_gen:                   # moved to another CSE -> resync with its command/la
                last, synced, gen = None, False, self.cse_gen
            if synced and self.notify_ok():
                time.sleep(1.0)
                continue
            try:
                c, js = self.req('GET', f'{self.ae}/command/la')
                cin = js.get('m2m:cin', {}) if c == 200 else {}
                ri = cin.get('ri')
                if synced and ri and ri != last:
                    self.accept_cin(cin, 'polling')
                if c in (200, 404):
                    if not synced and ri:
                        self.seen_ri.append(ri)
                    last, synced = ri or last, True
            except Exception as e:
                self.get_logger().warn(f'command polling error: {e}', throttle_duration_sec=10.0)
            time.sleep(1.0)

    def ensure_sub(self):
        # AE has rr=false, so notifications (incl. the verification request) are queued on the pollingChannel;
        # hence this runs in its own thread while pch_loop is polling.
        path = f'{self.ae}/command/{SUB_RN}'
        while not self.sub_ok:
            try:
                c, js = self.req('GET', path)
                if c == 200:
                    self.get_logger().info(f'Subscription {path} already exists -> reusing (nu={js.get("m2m:sub", {}).get("nu")})')
                    self.sub_ok = True
                    return
                c, js = self.req('POST', f'{self.ae}/command', 23, {'m2m:sub': {
                    'rn': SUB_RN, 'nu': [self.origin], 'nct': 1,           # nct 1 = full CIN in notification
                    'enc': {'net': [3]}}}, timeout=15)                     # net 3 = direct child created
                if c in (201, 409):
                    self.get_logger().info(f'Subscription {path} created -> commands via pollingChannel')
                    self.sub_ok = True
                    return
                self.get_logger().warn(f'Subscription create {c}: {str(js)[:200]} -> retrying in 10 s (polling meanwhile)')
            except Exception as e:
                self.get_logger().warn(f'Subscription create failed: {e} -> retrying in 10 s (polling meanwhile)')
            time.sleep(10.0)

    def find_pch(self):
        if self.req('GET', f'{self.ae}/{PCH_RN}', http=True)[0] == 200:
            return f'{self.ae}/{PCH_RN}'
        c, js = self.req('GET', f'{self.ae}?fu=1&ty=15', http=True)          # some CSEs allow only one pollingChannel per AE
        uril = js.get('m2m:uril', []) if c == 200 else []
        if isinstance(uril, dict):
            uril = uril.get('uril', [])
        if uril:
            name = str(uril[0]).rstrip('/').split('/')[-1]
            self.get_logger().info(f'pollingChannel already exists ({uril[0]}) -> reusing')
            return f'{self.ae}/{name}'
        c, js = self.req('POST', self.ae, 15, {'m2m:pch': {'rn': PCH_RN}}, http=True)
        if c in (201, 409):
            self.get_logger().info(f'pollingChannel {self.ae}/{PCH_RN} created')
            return f'{self.ae}/{PCH_RN}'
        raise RuntimeError(f'pollingChannel create {c}: {str(js)[:200]}')

    def cleanup_notify(self):
        # A leftover subscription blocks AE deletion on the CSE; the pollingChannel is kept for the next run.
        try:
            c, _ = self.req('DELETE', f'{self.ae}/command/{SUB_RN}', timeout=3)
            self.sub_ok = False
            print(f'[ipe] shutdown: subscription {self.ae}/command/{SUB_RN} deleted -> {c}', flush=True)
        except Exception as e:
            print(f'[ipe] shutdown: subscription delete failed ({e}). Delete it before deleting the AE', flush=True)

    def pch_loop(self):
        ready = False
        self.res_ready.wait()                         # the AE must exist before the pollingChannel
        gen = self.cse_gen
        while True:
            if gen != self.cse_gen:                   # moved to another CSE -> wait for its AE, then pch/subscription again
                gen, ready, self.pcu_ok = self.cse_gen, False, False
                self.res_ready.wait()
            try:
                if not ready:
                    self.pch_path = self.find_pch()
                    pcu = f'{self.pch_path}/pcu'
                    ready = True
                    self.pcu_ok = True
                    if not self.sub_ok:
                        threading.Thread(target=self.ensure_sub, daemon=True).start()
                c, js = self.req('GET', pcu, timeout=self.pch_wait_s, http=True)
                if c == 200 and js:
                    self.pcu_ok = True
                    self.on_pch(js)
                elif c == 404:
                    ready = self.pcu_ok = False
                elif c >= 400 and c not in (408, 504):  # 408/504 = long-poll expired with no notification (normal)
                    self.pcu_ok = False
                    self.get_logger().warn(f'pcu {c}: {str(js)[:200]}', throttle_duration_sec=30.0)
                    time.sleep(5.0)
            except requests.Timeout:
                pass
            except Exception as e:
                ready = self.pcu_ok = False
                self.get_logger().warn(f'pollingChannel error: {e} -> retrying in 5 s (polling meanwhile)',
                                       throttle_duration_sec=30.0)
                time.sleep(5.0)

    def on_pch(self, js):
        def find(o, key, out):
            if isinstance(o, dict):
                for k, v in o.items():
                    if k == key:
                        out.append(v)
                    else:
                        find(v, key, out)
            elif isinstance(o, list):
                for v in o:
                    find(v, key, out)
            return out
        for sgn in find(js, 'm2m:sgn', []):
            if sgn.get('vrq'):
                self.get_logger().info('Subscription verification request received -> OK')
            elif sgn.get('sud'):                     # deleted on purpose -> don't fight it by re-creating
                self.get_logger().warn('Subscription deleted notification -> polling for the rest of this run (re-created next run)')
                self.sub_ok = False
            else:
                for cin in find(sgn.get('nev', {}), 'm2m:cin', []):
                    self.accept_cin(cin, 'subscription')
        for rqp in find(js, 'm2m:rqp', []) or [js]:   # each request must be answered or the CSE times out
            rqi = rqp.get('rqi') if isinstance(rqp, dict) else None
            if not rqi:
                continue
            try:
                self.req('POST', f'{self.pch_path}/pcu',
                         body={'m2m:rsp': {'rsc': 2000, 'rqi': rqi, 'rvi': '3', 'fr': self.origin}}, http=True)
            except Exception as e:
                self.get_logger().warn(f'pcu response failed: {e}', throttle_duration_sec=30.0)

    def run_commands(self):
        while not self.cmds.empty():
            cmd = self.cmds.get_nowait()
            if not isinstance(cmd, dict) or not ('type' in cmd or 'command' in cmd):   # judge AE sends type, dashboard sends command
                self.get_logger().error(f'Invalid command (no type/command): {cmd}')
                continue
            # Interpreted by the demo script; the IPE only forwards it
            self.cmd_pub.publish(String(data=json.dumps(cmd, ensure_ascii=False)))
            self.get_logger().info(f'Command forwarded -> {self.topic_prefix}cmd: {cmd}')


def main():
    rclpy.init()
    node = None
    try:
        node = IPE()
        rclpy.spin(node)
    except KeyboardInterrupt:
        pass
    except Exception:                                 # ignore errors from the context closing first on Ctrl+C
        if rclpy.ok():
            raise
    finally:
        if node is not None:
            node.cleanup_notify()
            node.close_dump()


if __name__ == '__main__':
    import signal
    signal.signal(signal.SIGTERM, lambda *_: (_ for _ in ()).throw(KeyboardInterrupt()))   # route kill through cleanup
    main()
