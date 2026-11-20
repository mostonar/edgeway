# Site C pick/drop loop for site_c_AMR_2 (ROS_DOMAIN_ID=8) with a robot anomaly.
# Same moves as demo_b2_fl.py shifted to site C (x + 33.084). Once telemetry seq passes ANOMALY_SEQ
# the anomaly is turned on and the robot keeps the same route and states. Stops at END_SEQ.
# Anomalies are reset at start and end. The route does not cross the floor bumps.
#
# Run:   ROS_DOMAIN_ID=8 python -u demo_c2_fl.py [knob] [wknob] [bearing=N]
#   (none)      wknob: 2 knobs on the rear drive wheel (create them in Isaac first: isaac_script/make_wheel_lobe.py, MODE = "make")
#   knob        knob on the front roller (fades during in-place turns)
#   bearing=N   roller bearing friction, e.g. 60 (no bumps) or 10 (subtle)
# Needs: Isaac Sim playing with sim_watcher, Nav2 for domain 8 (Nav2/run_nav2.sh),
#        and the IPE for domain 8 (ROS-IPE/run_ipe.sh), required for the seq count.
# Knobs: ANOMALY_SEQ, END_SEQ, MAX_RUN_S (fallback timeout), CARGO_LOOP.

import fcntl, itertools, json, math, os, sys, time
import rclpy, tf2_ros
from rclpy.node import Node
from rclpy.action import ActionClient
from rclpy.time import Time
from action_msgs.msg import GoalStatus
from geometry_msgs.msg import Twist
from nav2_msgs.action import NavigateToPose
from nav2_msgs.srv import ClearEntireCostmap
from nav_msgs.msg import Odometry
from std_msgs.msg import Int64, String

ANOMALY_SEQ = 180                    # turn the anomaly on once telemetry seq passes this
END_SEQ = 300                        # finish once telemetry seq reaches this
MAX_RUN_S = 3600                     # fallback timeout if no seq arrives (s)
CARGO_LOOP = [150, 400, 50, 250, 100, 350, 200, 0, 250, 100, 400, 300]  # cargo per pick in the loop (kg), cycles
ROBOT = 'site_c_AMR_2'
ANOM_ON, ANOM_OFF = {}, {}
for a in sys.argv[1:]:
    if a in ('knob', 'wknob'):
        ANOM_ON[a], ANOM_OFF[a] = 'on', 'off'
    elif a.startswith('bearing='):
        ANOM_ON['bearing'], ANOM_OFF['bearing'] = a.split('=', 1)[1], '0'
if not ANOM_ON:
    ANOM_ON, ANOM_OFF = {'wknob': 'on'}, {'wknob': 'off'}   # default: rear-wheel knobs (keep bumping during turns)
ANOM_DESC = ' '.join(f'{k}={v}' for k, v in ANOM_ON.items())
RESET = {'knob': 'off', 'bearing': '0', **ANOM_OFF}  # also clears anomalies left on by a previous run

# Poses (x, y, yaw) of base_axle in the map frame. Spawn (domain 8): (59.45, -8.49), facing -y
ENTRY = (63.58, -3.0, math.pi / 2)   # right aisle entry
START_TURN_Y = -6.0                  # y of the sideways leg at start, below the shelves
LOAD1 = (63.58, 8.2, math.pi / 2)    # beside the middle shelf
END1 = (63.58, 15.0, math.pi / 2)    # aisle end
LOAD2 = (67.78, 9.4, -math.pi / 2)   # beside the right shelf, facing down
END2 = (67.78, 15.0, math.pi / 2)    # aisle end, right-shelf row
TOPIC_PREFIX = 'AMR/'                # same as fleet.yaml topic_prefix
CMD_VEL = '/cmd_vel_nav'
SIM_CMD, SIM_ACK = os.path.expanduser('~/sim_cmd'), os.path.expanduser('~/sim_ack')
SIM_LOCK = os.path.expanduser('~/sim_cmd.lock')  # sim_cmd is shared by all demo scripts
SOFT_DEG = 3.0                       # start turns at min speed for this many deg (lets the caster swivel)
SOFT_MAX_S = 3.0                     # ...but for at most this long (s)
PAUSE_S = 1.0                        # pause between actions (s)
LOOSE_DIST = 0.4                     # loose goal: arrived within this distance (m)
PASS_DIST = 1.5                      # waypoint: move on within this distance (m)
NEAR_BOX = (0.25, 0.8)               # pinpoint goal tolerance (x, y) in m; x tight for shelf clearance
FAIL_OK_DIST = 1.0                   # failed loose/waypoint goal still counts as arrived within this (m)
GOAL_RETRY = 2                       # resends of a failed goal
RETRY_WAIT_S = 3.0                   # wait before resending (s)


class Demo(Node):
    def __init__(self):
        super().__init__('demo_' + ROBOT)
        self.nav = ActionClient(self, NavigateToPose, 'navigate_to_pose')
        self.clear_cli = self.create_client(ClearEntireCostmap, 'local_costmap/clear_entirely_local_costmap')
        self._at = None
        self.vel = self.create_publisher(Twist, CMD_VEL, 10)
        self.state_pub = self.create_publisher(String, TOPIC_PREFIX + 'demo_state', 10)
        self.yaw = None
        self.create_subscription(Odometry, '/odom', self.on_odom, 10)
        self.tseq = 0
        self.anom_ready = False
        self.anom = False
        self.create_subscription(Int64, TOPIC_PREFIX + 'telemetry_seq', self.on_tseq, 10)
        self.tf = tf2_ros.Buffer()
        self.tf_listener = tf2_ros.TransformListener(self.tf, self)

    def state(self, s, **kw):
        self.check_anom()
        self.state_pub.publish(String(data=json.dumps({'state': s, **kw})))
        self.get_logger().info(f'state -> {s} {kw}')
        if s.split('/')[0] == 'done' or s == 'idle/aborted':   # resend the final state so the IPE gets it before exit
            for _ in range(3):
                time.sleep(0.3)
                self.state_pub.publish(String(data=json.dumps({'state': s, **kw})))

    def on_tseq(self, m):
        self.tseq = m.data

    def check_anom(self):
        if self.anom_ready and not self.anom and self.tseq >= ANOMALY_SEQ:
            if self.sim(**ANOM_ON):
                self.anom = True
                self.mark(f'robot anomaly on ({ANOM_DESC})')
            else:
                self.get_logger().error('failed to turn the robot anomaly on, retrying at the next action')

    def mark(self, msg):
        print(f'>>>>>>>>>> [c2 anomaly] seq {self.tseq}: {msg}', flush=True)

    def time_up(self):
        if time.time() - self.t0 > MAX_RUN_S:
            self.get_logger().warn(f'{MAX_RUN_S} s passed (seq {self.tseq}), finishing. Check that the IPE publishes telemetry_seq')
            return True
        return self.tseq >= END_SEQ

    def pause(self):
        time.sleep(PAUSE_S)

    def goto(self, x, y, yaw, pass_dist=None, loose=False, near=None, at=None):
        if near is None and not loose and not pass_dist:
            near = NEAR_BOX
        self._at = at
        for k in range(GOAL_RETRY + 1):
            st = self._goto_once(x, y, yaw, pass_dist, loose, near)
            if st is True:
                return True
            if st is None:   # no server or goal rejected: retrying will not help
                return False
            d_now = self.dist_now(x, y)
            if (near and self.in_box(x, y, near)) or \
               ((loose or pass_dist) and d_now is not None and d_now < FAIL_OK_DIST):
                self.get_logger().warn(f'goal failed but near ({x}, {y}) ({d_now:.2f} m left), treating as arrived')
                return True
            if k < GOAL_RETRY:
                self.get_logger().warn(f'goal failed, clearing local costmap and resending in {RETRY_WAIT_S:.0f} s ({k + 1}/{GOAL_RETRY})')
                time.sleep(RETRY_WAIT_S)
                self.clear_costmap()
        self.get_logger().error(f'goal ({x}, {y}) failed {GOAL_RETRY + 1} times')
        return False

    def _goto_once(self, x, y, yaw, pass_dist, loose, near):
        if not self.nav.wait_for_server(timeout_sec=10.0):
            self.get_logger().error('navigate_to_pose server not available')
            return None
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
            return None
        r = gh.get_result_async()
        while not r.done():
            rclpy.spin_once(self, timeout_sec=0.05)
            if self._at:
                d_at = self.dist_now(self._at[0], self._at[1])
                if d_at is not None and d_at < self._at[2]:
                    self.get_logger().info(f'passing ({self._at[0]}, {self._at[1]}), running scheduled action')
                    fn, self._at = self._at[3], None
                    fn()
            if near and self.in_box(x, y, near):
                self.stop_here(gh)
                self.get_logger().info(f'near ({x}, {y}), stopping here (heading is fixed by rotation)')
                return True
            d_now = self.dist_now(x, y)
            if pass_dist and d_now is not None and d_now < pass_dist:
                self.get_logger().info(f'passed waypoint ({x}, {y})')
                return True
            if loose and d_now is not None and d_now < LOOSE_DIST:
                self.stop_here(gh)
                self.get_logger().info(f'roughly at ({x}, {y}) ({d_now:.2f} m left)')
                return True
        if r.result().status == GoalStatus.STATUS_SUCCEEDED:
            return True
        self.get_logger().warn(f'goal failed status={r.result().status}')
        return False

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

    def clear_costmap(self):
        if not self.clear_cli.wait_for_service(timeout_sec=1.0):
            return
        f = self.clear_cli.call_async(ClearEntireCostmap.Request())
        rclpy.spin_until_future_complete(self, f, timeout_sec=2.0)

    def dist_now(self, x, y):
        try:
            p = self.tf.lookup_transform('map', 'base_axle', Time()).transform.translation
        except tf2_ros.TransformException:
            return None
        return math.hypot(x - p.x, y - p.y)

    def on_feedback(self, fb):
        self.remain = fb.feedback.distance_remaining

    def on_odom(self, m):
        q = m.pose.pose.orientation
        self.yaw = math.atan2(2 * (q.w * q.z + q.x * q.y), 1 - 2 * (q.y * q.y + q.z * q.z))

    def rotate(self, deg, wmax=0.6, wmin=0.15, tol_deg=1.0, timeout=60.0):   # wmax 0.6 avoids IMU spikes; timeout is wall-clock (slow sim)
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
                tw.angular.z = sign * wmin   # let the caster swivel first to avoid a jerk
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

    def goto_free(self, x, y):
        cur = None
        t_end = time.time() + 3.0
        while cur is None and time.time() < t_end:
            rclpy.spin_once(self, timeout_sec=0.05)
            try:
                cur = self.tf.lookup_transform('map', 'base_link', Time()).transform.translation
            except tf2_ros.TransformException:
                pass
        if cur is None:
            self.get_logger().error('no map->base_link TF, cannot move')
            return False
        return self.goto(x, y, math.atan2(y - cur.y, x - cur.x), loose=True)

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


class Finish(Exception):
    pass


def main():
    rclpy.init()
    d = Demo()
    time.sleep(1.0)
    d.t0 = time.time()
    cargo = itertools.cycle(CARGO_LOOP)
    where = 'END1'
    try:
        try:
            d.state('idle/reset', step=0)
            if not d.sim(cargo=0, **RESET):   # also clear anomalies left from a previous run
                return d.state('idle/aborted', step=0)

            d.pause()
            d.state('to_pick/turn_start', step=1)
            if not d.rotate_to(math.pi / 2, direction=+1):
                return d.state('idle/aborted', step=1)

            d.pause()
            d.state('to_pick', step=2)
            # right-angle path to the aisle entry (a diagonal S-curve spikes the IMU)
            if not d.goto(59.45, START_TURN_Y, math.pi / 2, loose=True):
                return d.state('idle/aborted', step=2)

            d.pause()
            d.state('to_pick/turn_side', step=2)
            if not d.rotate_to(0.0, direction=-1):
                return d.state('idle/aborted', step=2)

            d.pause()
            d.state('to_pick', step=2)
            if not d.goto(ENTRY[0], START_TURN_Y, 0.0, loose=True):
                return d.state('idle/aborted', step=2)

            d.pause()
            d.state('to_pick/turn_side', step=2)
            if not d.rotate_to(math.pi / 2, direction=+1):
                return d.state('idle/aborted', step=2)

            d.pause()
            d.state('to_pick', step=3)
            if not d.goto(*LOAD1):
                return d.state('idle/aborted', step=3)

            d.pause()
            d.state('to_pick/turn_in', step=4)
            if not d.rotate_to(0.0, direction=-1):
                return d.state('idle/aborted', step=4)

            d.pause()
            d.state('loading', step=5)
            if not d.sim(cargo=300):
                return d.state('idle/aborted', step=5)

            d.pause()
            d.state('to_drop/turn_out', step=6)
            if not d.rotate_to(math.pi / 2, direction=+1):
                return d.state('idle/aborted', step=6)

            d.pause()
            d.state('to_drop', step=7)
            if not d.goto(*END1, loose=True):
                return d.state('idle/aborted', step=7)

            d.pause()
            d.state('to_drop/turn_unload', step=8)
            if not d.rotate_to(-math.pi / 2, direction=+1):
                return d.state('idle/aborted', step=8)

            d.pause()
            d.state('unloading', step=9)
            if not d.sim(cargo=0):
                return d.state('idle/aborted', step=9)

            d.pause()
            lap = 0
            d.anom_ready = True
            while True:
                lap += 1
                where = 'END1'
                if d.time_up():
                    raise Finish
                if not d.anom and d.tseq >= ANOMALY_SEQ:
                    if not d.sim(**ANOM_ON):
                        return d.state('idle/aborted', step=11, lap=lap)
                    d.anom = True
                    d.mark(f'robot anomaly on ({ANOM_DESC})')
                d.state('to_pick/turn_side', step=10, lap=lap)
                if not d.rotate_to(0.0, direction=+1):
                    return d.state('idle/aborted', step=10, lap=lap)

                d.pause()
                d.state('to_pick', step=11, lap=lap)
                if not d.goto(LOAD2[0], END1[1], 0.0, loose=True):
                    return d.state('idle/aborted', step=11, lap=lap)

                d.pause()
                d.state('to_pick/turn_side', step=12, lap=lap)
                if not d.rotate_to(-math.pi / 2, direction=-1):
                    return d.state('idle/aborted', step=12, lap=lap)

                d.pause()
                d.state('to_pick', step=13, lap=lap)
                if not d.goto(*LOAD2):
                    return d.state('idle/aborted', step=13, lap=lap)

                d.pause()
                d.state('to_pick/turn_in', step=14, lap=lap)
                if not d.rotate_to(math.pi, direction=-1):
                    return d.state('idle/aborted', step=14, lap=lap)

                d.pause()
                d.state('loading', step=15, lap=lap)
                if not d.sim(cargo=next(cargo)):
                    return d.state('idle/aborted', step=15, lap=lap)

                d.pause()
                d.state('to_drop/turn_out', step=16, lap=lap)
                if not d.rotate_to(math.pi / 2, direction=-1):
                    return d.state('idle/aborted', step=16, lap=lap)

                d.pause()
                d.state('to_drop', step=17, lap=lap)
                if not d.goto(*END2, loose=True):
                    return d.state('idle/aborted', step=17, lap=lap)

                d.pause()
                d.state('to_drop/turn_unload', step=18, lap=lap)
                if not d.rotate_to(-math.pi / 2, direction=+1):
                    return d.state('idle/aborted', step=18, lap=lap)

                d.pause()
                d.state('unloading', step=19, lap=lap)
                if not d.sim(cargo=0):
                    return d.state('idle/aborted', step=19, lap=lap)

                d.pause()
                where = 'END2'
                if d.time_up():
                    raise Finish
                if not d.anom and d.tseq >= ANOMALY_SEQ:
                    if not d.sim(**ANOM_ON):
                        return d.state('idle/aborted', step=20, lap=lap)
                    d.anom = True
                    d.mark(f'robot anomaly on ({ANOM_DESC})')
                d.state('to_pick/turn_side', step=20, lap=lap)
                if not d.rotate_to(math.pi, direction=-1):
                    return d.state('idle/aborted', step=20, lap=lap)

                d.pause()
                d.state('to_pick', step=21, lap=lap)
                if not d.goto(LOAD1[0], END2[1], math.pi, loose=True):
                    return d.state('idle/aborted', step=21, lap=lap)

                d.pause()
                d.state('to_pick/turn_side', step=22, lap=lap)
                if not d.rotate_to(-math.pi / 2, direction=+1):
                    return d.state('idle/aborted', step=22, lap=lap)

                d.pause()
                d.state('to_pick', step=23, lap=lap)
                if not d.goto(LOAD1[0], LOAD1[1], -math.pi / 2):
                    return d.state('idle/aborted', step=23, lap=lap)

                d.pause()
                d.state('to_pick/turn_in', step=24, lap=lap)
                if not d.rotate_to(0.0, direction=+1):
                    return d.state('idle/aborted', step=24, lap=lap)

                d.pause()
                d.state('loading', step=25, lap=lap)
                if not d.sim(cargo=next(cargo)):
                    return d.state('idle/aborted', step=25, lap=lap)

                d.pause()
                d.state('to_drop/turn_out', step=26, lap=lap)
                if not d.rotate_to(math.pi / 2, direction=+1):
                    return d.state('idle/aborted', step=26, lap=lap)

                d.pause()
                d.state('to_drop', step=27, lap=lap)
                if not d.goto(*END1, loose=True):
                    return d.state('idle/aborted', step=27, lap=lap)

                d.pause()
                d.state('to_drop/turn_unload', step=28, lap=lap)
                if not d.rotate_to(-math.pi / 2, direction=+1):
                    return d.state('idle/aborted', step=28, lap=lap)

                d.pause()
                d.state('unloading', step=29, lap=lap)
                if not d.sim(cargo=0):
                    return d.state('idle/aborted', step=29, lap=lap)

                d.pause()
        except Finish:
            d.get_logger().info(f'telemetry seq {d.tseq} reached, finishing')
        d.sim(**RESET)   # clear the anomaly for the next run
        d.mark('done (anomaly off)')

        d.state('done', step=104)
    except KeyboardInterrupt:
        d.vel.publish(Twist())
    finally:
        time.sleep(1.0)
        d.destroy_node()
        rclpy.try_shutdown()


if __name__ == '__main__':
    main()