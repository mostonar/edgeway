# Robot anomaly scenario, cross-check robot: site_c_AMR_1 (site C, ROS_DOMAIN_ID=7).
# Idles until the suspect verdict on c2, drives over the suspect spot to cross-check, then on a final "robot"
# verdict (this robot is fine) loads at the right shelf and unloads at the aisle end. A "floor" verdict just ends the run.
# Run after reset, together with demo_c2_anno_robot.py:
#   ROS_DOMAIN_ID=7 python -u demo_c1_anno_robot.py
# Prerequisites: Isaac Sim playing with sim_watcher, floor bump removed (isaac_script/create_floor_bump.py, REMOVE = True),
#   Nav2 (nav_ws/run_nav2.sh 7 8), IPE (simple_ipe/run_ipe.sh 7 8).
# Verdicts arrive as oneM2M commands (judgment AE -> command CIN -> IPE -> AMR/cmd).
# Status: normal states while idle, cross_check (phase go / pass / hold) from the suspect to the final verdict.
import fcntl, json, math, os, time
import rclpy, tf2_ros
from rclpy.node import Node
from rclpy.action import ActionClient
from rclpy.time import Time
from action_msgs.msg import GoalStatus
from geometry_msgs.msg import Twist
from nav2_msgs.action import NavigateToPose
from nav2_msgs.srv import ClearEntireCostmap
from nav_msgs.msg import Odometry
from std_msgs.msg import String

ROBOT = 'site_c_AMR_1'
AE_ID = 'CsiteC_AMR_1'                # oneM2M AE-ID, compared with the verdict target
NEXT_CARGO = 200                     # kg, loaded after the robot verdict

# Poses (x, y, yaw) of base_axle in map; yaw: +x = 0, +y = pi/2. Spawn: base_axle (55.18, -8.49) facing -y.
BELOW_Y = -8.49                       # spawn row
NEAR_SHELF = (63.0, -5.6)             # waypoint past the middle shelf corner, keeps clear of the inspection corner
ENTRY_Y = -4.5                        # entry to the suspect lane, just below the shelves
VERIFY_END_Y = 9.4                    # stop beside the right shelf after passing the suspect spot
END_Y = 15.0                          # aisle end
TOPIC_PREFIX = 'AMR/'                # must match fleet.yaml topic_prefix
CMD_VEL = '/cmd_vel_nav'
SIM_CMD, SIM_ACK = os.path.expanduser('~/sim_cmd'), os.path.expanduser('~/sim_ack')
SIM_LOCK = os.path.expanduser('~/sim_cmd.lock')   # all demo scripts share one sim_cmd file
SOFT_DEG = 3.0                       # deg, turn at minimum speed first so the caster swivels
SOFT_MAX_S = 3.0                     # s, max soft-start time
PAUSE_S = 1.0                        # s, pause between actions
LOOSE_DIST = 0.4                     # m, loose goal arrival radius
PASS_DIST = 1.5                      # m, waypoint pass radius
NEAR_BOX = (0.25, 0.8)               # m (x, y), arrival box for pinpoint goals
FAIL_OK_DIST = 1.0                   # m, failed loose/waypoint goal within this counts as arrived
GOAL_RETRY = 2                       # resends of a failed goal
RETRY_WAIT_S = 3.0                   # s, wait before resending


class Demo(Node):
    def __init__(self):
        super().__init__('demo_' + ROBOT)
        self.nav = ActionClient(self, NavigateToPose, 'navigate_to_pose')
        self.clear_cli = self.create_client(ClearEntireCostmap, 'local_costmap/clear_entirely_local_costmap')
        self._at = None
        self.vel = self.create_publisher(Twist, CMD_VEL, 10)
        self.state_pub = self.create_publisher(String, TOPIC_PREFIX + 'demo_state', 10)
        self.cmd_box = []
        self.create_subscription(String, TOPIC_PREFIX + 'cmd', self.on_cmd, 10)
        self.event_pub = self.create_publisher(String, TOPIC_PREFIX + 'mock_event', 10)   # test-only hints
        self.yaw = None
        self.t_start = time.time()
        self.create_subscription(Odometry, '/odom', self.on_odom, 10)
        self.tf = tf2_ros.Buffer()
        self.tf_listener = tf2_ros.TransformListener(self.tf, self)

    def state(self, s, **kw):
        self.state_pub.publish(String(data=json.dumps({'state': s, **kw})))
        self.get_logger().info(f'State: {s}')
        if s.split('/')[0] == 'done' or s == 'idle/aborted':   # repeat the last state so IPE gets it before exit
            for _ in range(3):
                time.sleep(0.3)
                self.state_pub.publish(String(data=json.dumps({'state': s, **kw})))

    def event(self, **kv):
        self.event_pub.publish(String(data=json.dumps(kv)))

    def pause(self):
        time.sleep(PAUSE_S)

    def goto(self, x, y, yaw, pass_dist=None, loose=False, near=None, at=None):
        """Send a Nav2 goal and wait. pass_dist: waypoint; loose/near: early arrival; at=(x, y, r, fn): callback on the way."""
        if near is None and not loose and not pass_dist:
            near = NEAR_BOX
        self._at = at
        for k in range(GOAL_RETRY + 1):
            st = self._goto_once(x, y, yaw, pass_dist, loose, near)
            if st is True:
                return True
            if st is None:
                return False
            d_now = self.dist_now(x, y)
            if (near and self.in_box(x, y, near)) or \
               ((loose or pass_dist) and d_now is not None and d_now < FAIL_OK_DIST):
                return True
            if k < GOAL_RETRY:
                self.get_logger().warn(f'Goal failed, retrying in {RETRY_WAIT_S:.0f} s ({k + 1}/{GOAL_RETRY})')
                time.sleep(RETRY_WAIT_S)
                self.clear_costmap()
        self.get_logger().error(f'Goal ({x}, {y}) failed {GOAL_RETRY + 1} times')
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
            self.get_logger().error('Goal rejected')
            return None
        r = gh.get_result_async()
        while not r.done():
            rclpy.spin_once(self, timeout_sec=0.05)
            if self._at:
                d_at = self.dist_now(self._at[0], self._at[1])
                if d_at is not None and d_at < self._at[2]:
                    fn, self._at = self._at[3], None
                    fn()
            if near and self.in_box(x, y, near):
                self.stop_here(gh)
                return True
            d_now = self.dist_now(x, y)         # TF, not Nav2 feedback (stale right after a new goal)
            if pass_dist and d_now is not None and d_now < pass_dist:
                return True
            if loose and d_now is not None and d_now < LOOSE_DIST:
                self.stop_here(gh)
                return True
        if r.result().status == GoalStatus.STATUS_SUCCEEDED:
            return True
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

    def rotate(self, deg, wmax=1.0, wmin=0.15, tol_deg=1.0, timeout=20.0):
        """Rotate in place by deg (+ = left) using /odom yaw."""
        self.yaw = None
        t_end = time.time() + 5.0
        while self.yaw is None and time.time() < t_end:
            rclpy.spin_once(self, timeout_sec=0.1)
        if self.yaw is None:
            self.get_logger().error('No /odom, cannot rotate')
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
                # slow start until the swivel caster turns, avoids a jerk backwards
                tw.angular.z = sign * wmin
            else:
                tw.angular.z = sign * max(wmin, min(wmax, 1.5 * remain))
            self.vel.publish(tw)
        self.vel.publish(Twist())
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
        """Rotate in place to map yaw, turning left (+1) or right (-1)."""
        cur = self.map_yaw()
        if cur is None:
            self.get_logger().error('No map->base_link TF, cannot rotate')
            return False
        d = math.atan2(math.sin(yaw - cur), math.cos(yaw - cur))
        if direction > 0 and d < 0:
            d += 2 * math.pi
        elif direction < 0 and d > 0:
            d -= 2 * math.pi
        return self.rotate(math.degrees(d))

    def goto_free(self, x, y):
        # goal yaw = current heading to target, so the planner does not swing around to match a yaw
        cur = None
        t_end = time.time() + 3.0
        while cur is None and time.time() < t_end:
            rclpy.spin_once(self, timeout_sec=0.05)
            try:
                cur = self.tf.lookup_transform('map', 'base_link', Time()).transform.translation
            except tf2_ros.TransformException:
                pass
        if cur is None:
            self.get_logger().error('No map->base_link TF, cannot move')
            return False
        return self.goto(x, y, math.atan2(y - cur.y, x - cur.x), loose=True)

    def sim(self, **kv):
        """Send one line to sim_watcher and wait for the ACK with the same seq."""
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
                    return ack.split()[1] == 'ok'
                time.sleep(0.2)
        self.get_logger().error(f'No ACK from sim_watcher: {line}')
        return False

    # Verdict: {"type": "verdict", "case_id", "stage": "suspect" | "final", "site", "target": AE-ID,
    #           "cause": "robot" | "floor" (final only), "pos": {"x", "y"}}. Sent to every robot on the site.
    def on_cmd(self, m):
        try:
            c = json.loads(m.data)
        except ValueError:
            return self.get_logger().error(f'Command is not JSON: {m.data}')
        self.get_logger().info(f'\033[33mCommand received: {json.dumps(c, ensure_ascii=False)}\033[0m')
        self.cmd_box.append(c)

    def wait_cmd(self, match, timeout=1800.0):
        t_end = time.time() + timeout
        while time.time() < t_end:
            for c in self.cmd_box:
                if match(c):
                    self.cmd_box.remove(c)
                    return c
            rclpy.spin_once(self, timeout_sec=0.1)
        self.get_logger().error(f'Expected command did not arrive within {timeout:.0f} s')
        return None

    def wait_verdict(self, stage, mine, case=None, timeout=600.0):
        v = self.wait_cmd(lambda c: c.get('type') == 'verdict' and c.get('stage') == stage
                          and (c.get('target') == AE_ID) == mine and (case is None or c.get('case_id') == case), timeout)
        return v


def main():
    rclpy.init()
    d = Demo()
    time.sleep(1.0)
    try:
        # 0. reset: cargo 0, knob off
        d.state('idle/reset', step=0)
        if not d.sim(cargo=0, knob='off'):
            return d.state('idle/aborted', step=0)

        d.pause()

        # 1. idle until the suspect verdict on another robot
        d.state('idle', step=1)
        v = d.wait_verdict('suspect', mine=False, timeout=1800.0)
        if not v:
            return d.state('idle/aborted', step=1)
        px, py = v['pos']['x'], v['pos']['y']

        d.pause()

        # 2. turn left toward NEAR_SHELF
        d.state('cross_check', step=2, phase='go')
        if not d.rotate_to(math.atan2(NEAR_SHELF[1] - BELOW_Y, NEAR_SHELF[0] - 55.18), direction=+1):
            return d.state('idle/aborted', step=2)

        d.pause()

        # 3. via NEAR_SHELF to the suspect lane entry
        d.state('cross_check', step=3, phase='go')
        if not d.goto(*NEAR_SHELF, math.atan2(ENTRY_Y - NEAR_SHELF[1], px - NEAR_SHELF[0]), pass_dist=0.8) \
                or not d.goto_free(px, ENTRY_Y):
            return d.state('idle/aborted', step=3)

        d.pause()

        # 4. turn left to +y
        d.state('cross_check', step=4, phase='go')
        if not d.rotate_to(math.pi / 2, direction=+1):
            return d.state('idle/aborted', step=4)

        d.pause()

        # 5. drive up over the suspect spot, stop beside the right shelf
        d.state('cross_check', step=5, phase='pass', pos=[px, py])
        if not d.goto(px, VERIFY_END_Y, math.pi / 2, loose=True):
            return d.state('idle/aborted', step=5)

        d.pause()

        # 6. wait for the final verdict
        d.state('cross_check', step=6, phase='hold')
        v = d.wait_verdict('final', mine=False, case=v.get('case_id'))
        if not v:
            return d.state('idle/aborted', step=6)

        d.pause()

        if v.get('cause') != 'robot':              # floor verdict: must not pass the spot again, not covered here
            return d.state('done', step=7, cause=v.get('cause'))

        # 7. turn left to -x (rear faces the right shelf)
        d.state('to_pick/turn_in', step=7)
        if not d.rotate_to(math.pi, direction=+1):
            return d.state('idle/aborted', step=7)

        d.pause()

        # 8. load
        d.state('loading', step=8)
        if not d.sim(cargo=NEXT_CARGO):
            return d.state('idle/aborted', step=8)

        d.pause()

        # 9. turn right to +y
        d.state('to_drop/turn_out', step=9)
        if not d.rotate_to(math.pi / 2, direction=-1):
            return d.state('idle/aborted', step=9)

        d.pause()

        # 10. drive to the aisle end
        d.state('to_drop', step=10)
        if not d.goto(px, END_Y, math.pi / 2, loose=True):
            return d.state('idle/aborted', step=10)

        d.pause()

        # 11. turn 180 to -y (rear lift faces the end)
        d.state('to_drop/turn_unload', step=11)
        if not d.rotate_to(-math.pi / 2, direction=+1):
            return d.state('idle/aborted', step=11)

        d.pause()

        # 12. unload
        d.state('unloading', step=12)
        if not d.sim(cargo=0):
            return d.state('idle/aborted', step=12)

        d.pause()

        d.state('done', step=13)
    except KeyboardInterrupt:
        d.vel.publish(Twist())
    finally:
        time.sleep(1.0)
        d.destroy_node()
        rclpy.try_shutdown()


if __name__ == '__main__':
    main()
