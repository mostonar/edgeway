#!/bin/bash
# Stop all robots now: per domain, cancel all Nav2 goals and publish zero velocity for 2 s.
#   ./stop_all.sh          domains 1-8
#   ./stop_all.sh 2 4      domains 2 and 4 only
# Stop the demo scripts first (they would send new goals). Nav2 keeps running.
# Finishes within about CANCEL_T + PUB_T seconds per domain even if Nav2/Isaac are down.

DOMS="$*"
[ -z "$DOMS" ] && DOMS="1 2 3 4 5 6 7 8"

CANCEL_T=4      # s, cancel_goal service timeout
PUB_T=4         # s, zero-velocity publish timeout (2 s publish + ros2 startup)

# With --times, Humble's ros2 topic pub waits forever for a subscriber; disable that when supported
WAIT_OPT=""
ros2 topic pub -h 2>/dev/null | grep -q -- '--wait-matching-subscriptions' && WAIT_OPT="-w 0"

run() {  # run <timeout_s> <cmd...>; exit code 124 = timed out
  timeout -k 1 "$@" > /dev/null 2>&1
}

for d in $DOMS; do
  (
    export ROS_DOMAIN_ID=$d
    # Empty request = cancel all goals
    run $CANCEL_T ros2 service call /navigate_to_pose/_action/cancel_goal action_msgs/srv/CancelGoal "{}" &
    p1=$!
    # Zero both before (/cmd_vel_nav) and after (/cmd_vel) the velocity smoother, 20 Hz for 2 s
    run $PUB_T ros2 topic pub $WAIT_OPT -r 20 -t 40 /cmd_vel_nav geometry_msgs/msg/Twist "{}" &
    p2=$!
    run $PUB_T ros2 topic pub $WAIT_OPT -r 20 -t 40 /cmd_vel geometry_msgs/msg/Twist "{}" &
    p3=$!
    wait $p1; r1=$?; wait $p2; r2=$?; wait $p3; r3=$?
    st() { case $1 in 0) echo ok ;; 124|137) echo timeout ;; *) echo "failed($1)" ;; esac; }
    echo "Domain $d: cancel $(st $r1) / cmd_vel_nav $(st $r2) / cmd_vel $(st $r3)"
  ) &
done
wait
echo "All stopped"
