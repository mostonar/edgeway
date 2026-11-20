# Demo scenario for site_a_AMR_1 (site A, ROS_DOMAIN_ID=1).
# Pick at (0.08, 22.0), loop over the shelves via a waypoint into the left aisle, drop at (-18, -0.61),
# then run a short shuttle in front of home with varied cargo/speed and return home.
#
# Run:   ROS_DOMAIN_ID=1 python -u demo_a1_fl.py
# Needs: Isaac Sim playing with isaac_script/sim_watcher.py running,
#        Nav2 for this domain (Nav2/run_nav2.sh 1), IPE for this domain (ROS-IPE/run_ipe.sh 1).
# Knobs: CARGO_KG, SHUTTLE (cargo kg, speed m/s per trip), PAUSE_S.
import fcntl, json, math, os, time
import rclpy, tf2_ros
from rclpy.node import Node
from rclpy.action import ActionClient
from rclpy.time import Time
from action_msgs.msg import GoalStatus
from geometry_msgs.msg import Twist
from nav2_msgs.action import NavigateToPose
from nav_msgs.msg import Odometry
from std_msgs.msg import String
from rcl_interfaces.srv import SetParameters, GetParameters
from rcl_interfaces.msg import Parameter, ParameterValue, ParameterType

ROBOT = 'site_a_AMR_1'
CARGO_KG = 250   # kg (sim_watcher lift mass = 465 + cargo)
TOPIC_PREFIX = 'AMR/'   # must match topic_prefix in fleet.yaml
CMD_VEL = '/cmd_vel_nav'
SIM_CMD, SIM_ACK = os.path.expanduser('~/sim_cmd'), os.path.expanduser('~/sim_ack')
SIM_LOCK = os.path.expanduser('~/sim_cmd.lock')   # all demo scripts share one sim_cmd file
SOFT_DEG = 3.0   # deg turned at min speed first so the casters swivel
SOFT_MAX_S = 3.0   # s, cap on the soft start
PAUSE_S = 1.0   # s between actions

# Poses: (x, y, yaw) of base_axle in map, yaw 0 = +x. HOME = spawn pose.
WAYPOINT = (-4.73, 12.33, -math.pi / 2)
LOOSE_DIST = 0.4   # m, loose arrival radius
PASS_DIST = 1.5   # m, waypoint pass radius
PICK_NEAR = (0.25, 0.6)   # m (x, y) arrival box; x kept tight so the tail clears the shelf when turning
PRE_DROP = (-4.73, -0.61, -math.pi / 2)
DROP = (-18.0, -0.61, math.pi)
HOME = (4.72, DROP[1], 0.0)
# Slower empty return to reduce steering wobble; min lookahead is raised so it does not shrink with speed.
HOME_VEL, HOME_LA_MIN = 1.2, 1.8   # m/s, m for the idle/to_home leg
SPEED_PARAMS = ('FollowPath.desired_linear_vel', 'FollowPath.min_lookahead_dist')
# Extra data: shuttle in front of home, (cargo kg, speed m/s) per round trip. 0 kg = empty (to_pick).
SHUTTLE = [(400, 0.6), (0, 1.4), (300, 1.0), (150, 0.8), (0, 0.6)]
# 180 deg turns always swing the nose toward +y so they never overlap the neighbouring row.
SH_E, SH_W = 2.3, -1.7   # shuttle east/west end x


class Demo(Node):
    def __init__(self):
        super().__init__('demo_' + ROBOT)
        self.nav = ActionClient(self, NavigateToPose, 'navigate_to_pose')
        self.vel = self.create_publisher(Twist, CMD_VEL, 10)
        self.state_pub = self.create_publisher(String, TOPIC_PREFIX + 'demo_state', 10)
        self.yaw = None
        self.create_subscription(Odometry, '/odom', self.on_odom, 10)
        self.tf = tf2_ros.Buffer()
        self.tf_listener = tf2_ros.TransformListener(self.tf, self)
        self.ctrl_param = self.create_client(SetParameters, 'controller_server/set_parameters')
        self.ctrl_get = self.create_client(GetParameters, 'controller_server/get_parameters')
        self.speed_orig = None   # read from the running Nav2 on first slow_down

    # state = base state (idle/to_pick/loading/to_drop/unloading/done) + optional '/detail'
    def state(self, s, **kw):
        self.state_pub.publish(String(data=json.dumps({'state': s, **kw})))
        self.get_logger().info(f'state -> {s} {kw}')
        if s.split('/')[0] == 'done' or s == 'idle/aborted':   # resend so the IPE gets it before exit
            for _ in range(3):
                time.sleep(0.3)
                self.state_pub.publish(String(data=json.dumps({'state': s, **kw})))

    def pause(self):
        time.sleep(PAUSE_S)

    def goto(self, x, y, yaw, pass_dist=None, loose=False, near=None):
        if not self.nav.wait_for_server(timeout_sec=10.0):
            self.get_logger().error('navigate_to_pose server not available')
            return False
        g = NavigateToPose.Goal()
        g.pose.header.frame_id = 'map'
        g.pose.pose.position.x, g.pose.pose.position.y = x, y
        g.pose.pose.orientation.z, g.pose.pose.orientation.w = math.sin(yaw / 2), math.cos(yaw / 2)
        self.remain = None
        f = self.nav.send_goal_async(g, feedback_callback=self.on_feedback)
        rclpy.spin_until_future_complete(self, f)
        gh = f.result()
        if not gh.accepted:
            self.get_logger().error('goal rejected')
            return False
        r = gh.get_result_async()
        while not r.done():
            rclpy.spin_once(self, timeout_sec=0.05)
            if near and self.in_box(x, y, near):
                self.stop_here(gh)
                self.get_logger().info(f'near ({x}, {y}), stopping here (heading set by rotation)')
                return True
            d_now = self.dist_now(x, y)   # TF distance; Nav2 feedback can be stale right after a new goal
            if pass_dist and d_now is not None and d_now < pass_dist:
                self.get_logger().info(f'passed waypoint ({x}, {y})')
                return True
            if loose and d_now is not None and d_now < LOOSE_DIST:
                cf = gh.cancel_goal_async()
                rclpy.spin_until_future_complete(self, cf, timeout_sec=2.0)
                for _ in range(5):
                    self.vel.publish(Twist())
                    time.sleep(0.05)
                time.sleep(0.5)
                self.get_logger().info(f'roughly at ({x}, {y}) ({d_now:.2f} m left)')
                return True
        ok = r.result().status == GoalStatus.STATUS_SUCCEEDED
        if not ok and near and self.in_box(x, y, near):
            self.get_logger().warn(f'goal failed status={r.result().status}, but near ({x}, {y}); treating as arrived')
            return True
        if not ok:
            self.get_logger().error(f'goal failed status={r.result().status}')
        return ok

    def in_box(self, x, y, near):
        try:
            p = self.tf.lookup_transform('map', 'base_axle', Time()).transform.translation
        except tf2_ros.TransformException:
            return False
        return abs(p.x - x) <= near[0] and abs(p.y - y) <= near[1]

    def stop_here(self, gh):
        cf = gh.cancel_goal_async()
        rclpy.spin_until_future_complete(self, cf, timeout_sec=2.0)
        for _ in range(5):
            self.vel.publish(Twist())
            time.sleep(0.05)
        time.sleep(0.5)

    def dist_now(self, x, y):
        try:
            p = self.tf.lookup_transform('map', 'base_axle', Time()).transform.translation
        except tf2_ros.TransformException:
            return None
        return math.hypot(x - p.x, y - p.y)

    def set_speed(self, vel, la_min):
        if not self.ctrl_param.wait_for_service(timeout_sec=2.0):
            self.get_logger().warn('controller_server/set_parameters not available, speed unchanged')
            return False
        req = SetParameters.Request()
        for name, v in zip(SPEED_PARAMS, (vel, la_min)):
            req.parameters.append(Parameter(name=name, value=ParameterValue(type=ParameterType.PARAMETER_DOUBLE, double_value=float(v))))
        f = self.ctrl_param.call_async(req)
        rclpy.spin_until_future_complete(self, f, timeout_sec=2.0)
        res = f.result()
        if res is None or not all(r.successful for r in res.results):
            self.get_logger().warn(f'speed change failed: {res}')
            return False
        self.get_logger().info(f'speed {vel} m/s, min lookahead {la_min} m')
        return True

    def read_speed(self):
        if not self.ctrl_get.wait_for_service(timeout_sec=2.0):
            return None
        f = self.ctrl_get.call_async(GetParameters.Request(names=list(SPEED_PARAMS)))
        rclpy.spin_until_future_complete(self, f, timeout_sec=2.0)
        res = f.result()
        if res is None or len(res.values) != 2 or any(v.type != ParameterType.PARAMETER_DOUBLE for v in res.values):
            return None
        return tuple(v.double_value for v in res.values)

    def slow_down(self, vel=HOME_VEL):
        if self.speed_orig is None:
            self.speed_orig = self.read_speed()
            if self.speed_orig is None:
                self.get_logger().warn('could not read original speed, not slowing down')
                return False
            self.get_logger().info(f'original speed {self.speed_orig[0]} m/s, min lookahead {self.speed_orig[1]} m')
        return self.set_speed(vel, HOME_LA_MIN)

    def restore_speed(self):
        if self.speed_orig is not None:
            self.set_speed(*self.speed_orig)

    def on_feedback(self, fb):
        self.remain = fb.feedback.distance_remaining

    def on_odom(self, m):
        q = m.pose.pose.orientation
        self.yaw = math.atan2(2 * (q.w * q.z + q.x * q.y), 1 - 2 * (q.y * q.y + q.z * q.z))

    def rotate(self, deg, wmax=0.6, wmin=0.15, tol_deg=1.0, timeout=60.0):   # timeout is wall-clock s (slow sim)
        self.yaw = None
        t_end = time.time() + 5.0
        while self.yaw is None and time.time() < t_end:
            rclpy.spin_once(self, timeout_sec=0.1)
        if self.yaw is None:
            self.get_logger().error('no /odom, cannot rotate')
            return False
        target, turned, prev = math.radians(abs(deg)), 0.0, self.yaw
        sign = 1.0 if deg > 0 else -1.0
        tw = Twist()
        t_end = time.time() + timeout
        t_soft = time.time() + SOFT_MAX_S
        while time.time() < t_end:
            rclpy.spin_once(self, timeout_sec=0.05)
            d = self.yaw - prev
            turned += math.atan2(math.sin(d), math.cos(d)) * sign
            prev = self.yaw
            remain = target - turned
            if remain <= math.radians(tol_deg):
                break
            if turned < math.radians(SOFT_DEG) and time.time() < t_soft:
                tw.angular.z = sign * wmin   # soft start so the casters swivel first
            else:
                tw.angular.z = sign * max(wmin, min(wmax, 1.5 * remain))
            self.vel.publish(tw)
        self.vel.publish(Twist())
        self.get_logger().info(f'rotated {math.degrees(turned):.1f} deg / target {abs(deg)} deg')
        return abs(target - turned) < math.radians(5.0)

    def map_yaw(self):
        t_end = time.time() + 3.0
        while time.time() < t_end:
            rclpy.spin_once(self, timeout_sec=0.05)
            try:
                q = self.tf.lookup_transform('map', 'base_link', Time()).transform.rotation
                return math.atan2(2 * (q.w * q.z + q.x * q.y), 1 - 2 * (q.y * q.y + q.z * q.z))
            except tf2_ros.TransformException:
                pass
        return None

    def rotate_to(self, yaw, direction=+1):
        cur = self.map_yaw()
        if cur is None:
            self.get_logger().error('no map->base_link TF, cannot rotate')
            return False
        d = math.atan2(math.sin(yaw - cur), math.cos(yaw - cur))
        if direction > 0 and d < 0:
            d += 2 * math.pi
        elif direction < 0 and d > 0:
            d -= 2 * math.pi
        self.get_logger().info(f'map yaw {math.degrees(cur):.1f} -> {math.degrees(yaw):.1f} deg ({math.degrees(d):+.1f} deg)')
        return self.rotate(math.degrees(d))

    def sim(self, **kv):
        seq = str(time.time_ns())
        line = f'seq={seq} robot={ROBOT} ' + ' '.join(f'{k}={v}' for k, v in kv.items())
        with open(SIM_LOCK, 'w') as lk:
            fcntl.flock(lk, fcntl.LOCK_EX)
            with open(SIM_CMD, 'w') as f:
                f.write(line + '\n')
            t_end = time.time() + 10.0
            while time.time() < t_end:
                try:
                    ack = open(SIM_ACK).read().strip()
                except OSError:
                    ack = ''
                if ack.startswith(f'seq={seq} '):
                    self.get_logger().info(f'sim: {line} -> {ack}')
                    return ack.split()[1] == 'ok'
                time.sleep(0.2)
        self.get_logger().error(f'no sim ACK (is sim_watcher running?): {line}')
        return False


def main():
    rclpy.init()
    d = Demo()
    time.sleep(1.0)
    try:
        d.state('idle/reset', step=0)
        if not d.sim(cargo=0):
            return d.state('idle/aborted', step=0)

        d.pause()

        d.state('to_pick', step=1)
        if not d.goto(0.08, 22.0, -math.pi / 2, near=PICK_NEAR):
            return d.state('idle/aborted', step=1)

        d.pause()

        d.state('to_pick/turn_in', step=2)
        if not d.rotate_to(0.0, direction=+1):
            return d.state('idle/aborted', step=2)

        d.pause()

        d.state('loading', step=3)
        if not d.sim(cargo=CARGO_KG):
            return d.state('idle/aborted', step=3)

        d.pause()

        d.state('to_drop/turn_out', step=4)
        if not d.rotate_to(math.pi / 2, direction=+1):
            return d.state('idle/aborted', step=4)

        d.pause()

        d.state('to_drop', step=5)
        if not d.goto(*WAYPOINT, pass_dist=PASS_DIST):
            return d.state('idle/aborted', step=5)

        d.state('to_drop', step=6)
        if not d.goto(*PRE_DROP, loose=True):
            return d.state('idle/aborted', step=6)

        d.pause()

        d.state('to_drop/turn_drop', step=7)
        if not d.rotate_to(math.pi, direction=-1):
            return d.state('idle/aborted', step=7)

        d.pause()

        d.state('to_drop', step=8)
        if not d.goto(*DROP, loose=True):
            return d.state('idle/aborted', step=8)

        d.pause()

        d.state('to_drop/turn_unload', step=9)
        if not d.rotate_to(DROP[2] + math.pi):
            return d.state('idle/aborted', step=9)

        d.pause()

        d.state('unloading', step=10)
        if not d.sim(cargo=0):
            return d.state('idle/aborted', step=10)

        d.pause()

        d.state('idle/to_home', step=11)
        d.slow_down()
        ok = d.goto(SH_E, HOME[1], 0.0, loose=True)   # shuttle east end first; HOME comes after the shuttle
        d.restore_speed()
        if not ok:
            return d.state('idle/aborted', step=11)

        d.pause()

        # shuttle for extra data
        for trip, (kg, vel) in enumerate(SHUTTLE, start=1):
            lab = 'to_drop' if kg else 'to_pick'

            d.state('loading' if kg else 'unloading', step=12, trip=trip, kg=kg)
            if not d.sim(cargo=kg):
                return d.state('idle/aborted', step=12)

            d.pause()

            d.state(lab + '/turn_start', step=13, trip=trip)
            if not d.rotate_to(math.pi, direction=+1):
                return d.state('idle/aborted', step=13)

            d.pause()

            d.state(lab, step=14, trip=trip, vel=vel)
            d.slow_down(vel)
            ok = d.goto(SH_W, HOME[1], math.pi, loose=True)
            d.restore_speed()
            if not ok:
                return d.state('idle/aborted', step=14)

            d.pause()

            d.state(lab + '/turn_start', step=15, trip=trip)
            if not d.rotate_to(0.0, direction=-1):
                return d.state('idle/aborted', step=15)

            d.pause()

            d.state(lab, step=16, trip=trip, vel=vel)
            d.slow_down(vel)
            ok = d.goto(SH_E, HOME[1], 0.0, loose=True)
            d.restore_speed()
            if not ok:
                return d.state('idle/aborted', step=16)

            d.pause()

        d.state('unloading', step=17)
        if not d.sim(cargo=0):
            return d.state('idle/aborted', step=17)
        d.pause()
        d.state('idle/to_home', step=17)
        d.slow_down()
        ok = d.goto(*HOME, loose=True)
        d.restore_speed()
        if not ok:
            return d.state('idle/aborted', step=17)

        d.pause()

        d.state('done', step=18)
    except KeyboardInterrupt:
        d.vel.publish(Twist())
    finally:
        try:
            d.restore_speed()
        except Exception:
            pass
        time.sleep(1.0)
        d.destroy_node()
        rclpy.try_shutdown()


if __name__ == '__main__':
    main()