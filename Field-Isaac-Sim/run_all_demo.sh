#!/bin/bash
# Start Nav2 -> IPE -> the 8 FL data-collection demos, each stage only after the previous one is ready.
#   ./run_all_demo.sh         run (after Isaac Play + sim_watcher are running)
#   ./run_all_demo.sh stop    only clean up leftovers from a previous run
# Demo output goes to this terminal; Nav2 and IPE write to their own logs/ folders.
# Ctrl+C: demos -> stop robots (stop_all.sh) -> IPE -> Nav2 -> kill leftovers. Each step has a timeout.
# Press Ctrl+C again during cleanup to kill everything immediately.
# If this script itself is killed with -9, run ./run_all_demo.sh stop.

ROOT="$(cd "$(dirname "$0")" && pwd)"
NAV=$ROOT/nav_ws/run_nav2.sh
STOP=$ROOT/nav_ws/stop_all.sh
IPE=$ROOT/ros_ipe/run_ipe.sh
IPE_LOG=$ROOT/ros_ipe/logs
DEMO_DIR=$ROOT/demo_task_executor
DEMO_GAP=1           # s between demo starts
NAV_TIMEOUT=180      # s to wait for all Nav2 to become active
IPE_TIMEOUT=120      # s to wait for all IPEs to be ready

P_DEMO='python[^ ]* .*demo_'
P_IPE='ipe_proc'
P_NAV='component_container|bringup_launch|static_transform_publisher'

#  domain  script
DEMOS="
   1      demo_a1_fl.py
   2      demo_a2_fl.py
   3      demo_a3_fl.py
   4      demo_a4_fl.py
   5      demo_b1_fl.py
   6      demo_b2_fl.py
   7      demo_c1_fl.py
   8      demo_c2_fl.py
"
DOMS=$(awk 'NF {print $1}' <<< "$DEMOS" | tr '\n' ' ')

NAV_PID=; IPE_PID=; DEMO_PIDS=(); CLEANING=0; FORCE=0

# Run in background + wait so Ctrl+C is handled immediately (bash runs traps only after a foreground command ends)
nap() { sleep "$1" & wait $! 2>/dev/null; }
bg_out() { "$@" > /tmp/run_all.$$ 2>/dev/null & local p=$!; wait $p 2>/dev/null; local rc=$?; kill $p 2>/dev/null; cat /tmp/run_all.$$ 2>/dev/null; return $rc; }

# wait_gone <sec> <pgrep pattern>: 0 once no process matches
wait_gone() {
  local t=$1 pat=$2
  for _ in $(seq $((t * 2))); do
    pgrep -f "$pat" > /dev/null || return 0
    [ $FORCE = 1 ] && return 1
    sleep 0.5
  done
  return 1
}

# stop_step <name> <sec> <pgrep pattern> <signal> [process group PID]: signal -> wait -> KILL
stop_step() {
  local name=$1 t=$2 pat=$3 sig=$4 grp=$5
  [ -n "$grp" ] && kill -$sig -- -"$grp" 2>/dev/null
  pkill -$sig -f "$pat" 2>/dev/null
  if wait_gone "$t" "$pat"; then
    echo "  $name: stopped"
  elif [ $sig != TERM ] && [ $FORCE = 0 ] && { [ -n "$grp" ] && kill -TERM -- -"$grp" 2>/dev/null; pkill -TERM -f "$pat" 2>/dev/null; wait_gone 5 "$pat"; }; then
    echo "  $name: still running after ${t}s, stopped with TERM"
  else
    echo "  $name: still running -> KILL"
    [ -n "$grp" ] && kill -KILL -- -"$grp" 2>/dev/null
    pkill -KILL -f "$pat" 2>/dev/null
    wait_gone 5 "$pat" || echo "  ! $name: still running after KILL"
  fi
}

cleanup() {
  [ $CLEANING = 1 ] && return
  CLEANING=1
  trap 'FORCE=1; echo "  (Ctrl+C again -> killing the rest immediately)"' INT
  trap '' TERM HUP
  echo; echo "=== Shutdown: demos -> stop robots -> IPE -> Nav2 ==="
  # Demos first: while alive they keep sending goals. INT runs their finally blocks (restore speed etc.)
  for p in "${DEMO_PIDS[@]}"; do kill -INT "$p" 2>/dev/null; done
  stop_step "Demos" 5 "$P_DEMO" INT
  # Robot stop needs Nav2 alive to cancel goals
  if [ $FORCE = 1 ]; then
    echo "  Stop robots: skipped (force mode)"
  elif [ -x "$STOP" ]; then
    echo "  Stop robots: $STOP"
    timeout 20 "$STOP" > /dev/null 2>&1 || echo "  ! stop_all.sh did not finish in 20s (skipped)"
  else
    echo "  ! $STOP not found -> skipping robot stop"
  fi
  # IPE ignores INT (started in background by run_ipe.sh); on TERM it removes its subscriptions, so give it time
  stop_step "IPE" 15 "$P_IPE" TERM "$IPE_PID"
  stop_step "Nav2" 20 "$P_NAV" INT "$NAV_PID"
  left=$(pgrep -af "$P_DEMO|$P_IPE|$P_NAV")
  if [ -n "$left" ]; then
    echo "=== ! Processes still running ==="; echo "$left"
  else
    echo "=== All stopped ==="
  fi
  rm -f /tmp/run_all.$$
  exit 0
}
[ "$1" = "stop" ] && { trap cleanup INT TERM HUP; cleanup; }

[ -x "$NAV" ] || { echo "! $NAV missing (or not executable)"; exit 1; }
[ -x "$IPE" ] || { echo "! $IPE missing (or not executable)"; exit 1; }
while read -r dom f; do
  [ -z "$dom" ] && continue
  [ -f "$DEMO_DIR/$f" ] || { echo "! Demo not found: $DEMO_DIR/$f"; exit 1; }
done <<< "$DEMOS"
left=$(pgrep -af "$P_DEMO|$P_IPE|$P_NAV")
if [ -n "$left" ]; then
  echo "! Previous run still alive -> run ./run_all_demo.sh stop first"; echo "$left"; exit 1
fi

# From here on, always clean up (not before: the checks above must not touch processes started by others)
trap cleanup INT TERM HUP
trap cleanup EXIT

echo "=== 1/3 Starting Nav2 ==="
setsid "$NAV" &
NAV_PID=$!
echo "Waiting for bt_navigator to be active in each domain (max ${NAV_TIMEOUT}s)..."
t0=$(date +%s)
for d in $DOMS; do
  until ROS_DOMAIN_ID=$d bg_out timeout 10 ros2 lifecycle get /bt_navigator | grep -q '^active'; do
    kill -0 "$NAV_PID" 2>/dev/null || { echo "! run_nav2.sh exited"; cleanup; }
    [ $(( $(date +%s) - t0 )) -gt $NAV_TIMEOUT ] && { echo "! Domain $d: Nav2 not active within ${NAV_TIMEOUT}s (see $ROOT/nav_ws/logs/nav2_$d.log)"; cleanup; }
    nap 2
  done
  echo "  Domain $d: Nav2 active"
done
echo "=== Nav2 ready ($(( $(date +%s) - t0 ))s) ==="

echo "=== 2/3 Starting IPE ==="
setsid "$IPE" &
IPE_PID=$!
nap 2                       # let run_ipe.sh recreate the log files so old logs are not read
echo "Waiting for 'oneM2M resources ready' in each domain (max ${IPE_TIMEOUT}s)..."
t0=$(date +%s)
for d in $DOMS; do
  until grep -q 'oneM2M resources ready' "$IPE_LOG/ipe_$d.log" 2>/dev/null; do
    kill -0 "$IPE_PID" 2>/dev/null || { echo "! run_ipe.sh exited"; cleanup; }
    [ $(( $(date +%s) - t0 )) -gt $IPE_TIMEOUT ] && {
      echo "! Domain $d: IPE not ready within ${IPE_TIMEOUT}s. Log tail:"; tail -5 "$IPE_LOG/ipe_$d.log"; cleanup; }
    nap 1
  done
  echo "  Domain $d: IPE ready"
done
echo "=== IPE ready ($(( $(date +%s) - t0 ))s) ==="

echo "=== 3/3 Starting demos (${DEMO_GAP}s apart) ==="
# Background jobs in a script ignore INT unless job control is on; set -m gives each demo its own
# process group so terminal Ctrl+C reaches only this script and cleanup stops the demos in order.
set -m
while read -r dom f; do
  [ -z "$dom" ] && continue
  ROS_DOMAIN_ID=$dom python -u "$DEMO_DIR/$f" &
  DEMO_PIDS+=($!)
  echo "Domain $dom: started $f"
  nap "$DEMO_GAP"
done <<< "$DEMOS"
set +m

wait "${DEMO_PIDS[@]}"
echo "=== All demos finished. Nav2 and IPE still running -> Ctrl+C to stop ==="
while true; do nap 1; done     # Nav2/IPE are in other sessions, so plain wait could return immediately
