#!/bin/bash
# Start one IPE (ipe_proc.py) per robot (ROS domain).
#   ./run_ipe.sh           all robots in fleet.yaml
#   ./run_ipe.sh 7 8       domains 7 and 8 only
# Needs fleet.yaml (copy from fleet.example.yaml and set cse_url). Logs: logs/ipe_<domain>.log
# Ctrl+C stops everything.

cd "$(dirname "$0")" && mkdir -p logs
SCRIPT=ipe_proc.py
DOMS="$*"
[ -z "$DOMS" ] && DOMS=$(python3 -c "import yaml; print(*yaml.safe_load(open('fleet.yaml'))['robots'])")

PIDS=()
cleanup() {
  echo; echo "Stopping..."
  for p in "${PIDS[@]}"; do kill -INT -- -"$p" 2>/dev/null; done
  sleep 3
  for p in "${PIDS[@]}"; do kill -KILL -- -"$p" 2>/dev/null; done
  echo "Stopped"; exit 0
}
trap cleanup INT TERM

for d in $DOMS; do
  ROS_DOMAIN_ID=$d setsid python -u "$SCRIPT" --ros-args --params-file ipe_common.yaml \
    -p use_sim_time:=true -p fleet_file:="$PWD/fleet.yaml" > "logs/ipe_$d.log" 2>&1 &
  PIDS+=($!)
  echo "Domain $d: $SCRIPT started  log: logs/ipe_$d.log"
done

sleep 3
i=0; for d in $DOMS; do
  kill -0 "${PIDS[$i]}" 2>/dev/null || { echo "! Domain $d: IPE exited. Log tail:"; tail -5 "logs/ipe_$d.log"; }
  i=$((i+1))
done
echo "Running. Ctrl+C to stop all."
wait
