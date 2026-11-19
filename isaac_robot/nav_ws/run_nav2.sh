#!/bin/bash
# Start one Nav2 stack per robot (ROS domain).
#   ./run_nav2.sh          all robots in ROBOTS
#   ./run_nav2.sh 7 8      domains 7 and 8 only
# Start Isaac Play first. Ctrl+C stops everything. Logs and per-robot params go to logs/.
# Start poses are the Isaac base_link poses (measured in Stop state); the script converts them to
# base_axle (AXLE_OFFSET behind base_link) for the AMCL initial pose.
# Uses the patched launch files in launch/ (created by patch_nav2_launch.sh on first run).

PARAMS=nav2_drive_param.yaml
AXLE_OFFSET=0.90   # m, base_link -> base_axle (backwards)

#  domain  map                x       y        yaw(deg)   <- base_link start pose
ROBOTS="
   1      maps/siteA.yaml    5.62    -0.61    0
   2      maps/siteA.yaml    5.62    -4.07    0
   3      maps/siteA.yaml    5.62    -7.53    0
   4      maps/siteA.yaml    5.62    -10.99   0
   5      maps/siteB.yaml    21.23   -9.39    -90
   6      maps/siteB.yaml    24.85   -9.39    -90
   7      maps/siteC.yaml    55.18   -9.39    -90
   8      maps/siteC.yaml    59.45   -9.39    -90
"

cd "$(dirname "$0")" && mkdir -p logs
if [ ! -f launch/bringup_launch.py ]; then
  ./patch_nav2_launch.sh || { echo "! Failed to create patched launch files"; exit 1; }
fi
PIDS=()
cleanup() {
  echo; echo "Stopping... (up to 20s)"
  for p in "${PIDS[@]}"; do kill -INT -- -"$p" 2>/dev/null; done
  for i in $(seq 20); do
    alive=0; for p in "${PIDS[@]}"; do kill -0 "$p" 2>/dev/null && alive=1; done
    [ $alive = 0 ] && break; sleep 1
  done
  for p in "${PIDS[@]}"; do kill -KILL -- -"$p" 2>/dev/null; done
  pgrep -af component_container && echo "^ Nav2 processes still running" || echo "Stopped"
  exit 0
}
trap cleanup INT TERM

while read -r dom map x y yaw; do
  [ -z "$dom" ] && continue
  [ $# -gt 0 ] && [[ ! " $* " =~ " $dom " ]] && continue
  [ -f "$map" ] || { echo "! Domain $dom: map not found ($map)"; continue; }

  # Per-robot params: AMCL initial pose (base_axle) and absolute BT xml path
  P="logs/params_$dom.yaml"
  AXLE=$(python3 - "$PARAMS" "$P" "$x" "$y" "$yaw" "$AXLE_OFFSET" <<'PY'
import sys, math, yaml
src, dst = sys.argv[1], sys.argv[2]
x, y, yaw_deg, off = map(float, sys.argv[3:])
yaw = math.radians(yaw_deg)
ax = x - off * math.cos(yaw)
ay = y - off * math.sin(yaw)
p = yaml.safe_load(open(src))
a = p['amcl']['ros__parameters']
a['set_initial_pose'] = True
a['initial_pose'] = {'x': round(ax, 3), 'y': round(ay, 3), 'z': 0.0, 'yaw': yaw}
p['bt_navigator']['ros__parameters']['default_nav_to_pose_bt_xml'] = __import__('os').path.abspath('nav_to_pose_no_spin.xml')
yaml.safe_dump(p, open(dst, 'w'), sort_keys=False)
print(f'{ax:.2f} {ay:.2f}')
PY
  ) || { echo "! Domain $dom: failed to create params"; continue; }

  # Static TF base_link -> base_axle, before Nav2 (used by AMCL and costmaps)
  ROS_DOMAIN_ID=$dom setsid ros2 run tf2_ros static_transform_publisher \
    --x "-$AXLE_OFFSET" --frame-id base_link --child-frame-id base_axle \
    > "logs/static_tf_$dom.log" 2>&1 &
  PIDS+=($!)
  sleep 1

  ROS_DOMAIN_ID=$dom setsid ros2 launch "$PWD/launch/bringup_launch.py" use_sim_time:=True \
    map:="$PWD/$map" params_file:="$PWD/$P" > "logs/nav2_$dom.log" 2>&1 &
  PIDS+=($!)
  echo "Domain $dom: Nav2 started ($map, base_link $x $y ${yaw}deg -> base_axle $AXLE)  log: logs/nav2_$dom.log"
  sleep 3
done <<< "$ROBOTS"

echo "Running. Ctrl+C to stop all."
wait
