#!/bin/bash
# run_local_model.sh — start the 3 site apps + judge apps at once
#
#   bash run_local_model.sh       (can be run from any directory)
#
# To start them one by one, use run_site_app.sh A|B|C and run_judge.sh A|B|C.
# This script does the common preparation (cleanup, MN-CSE resources, waiting for robots) once
# and then calls those two with FL_STANDALONE=0.
#
# Order
#   Clean up previous run → check/create MN-CSE resources → wait for Isaac Sim robots
#   → 3 site apps → judge apps (JUDGE_SITES)
#
# Prerequisites: .env.local has FL_SANDBOX_HOST / FL_SANDBOX_NAME, FL_IN_BASE_URL /
#   FL_IN_CSE_NAME, FL_NOTIFY_HOST. See README.md for details.
#
# Common settings
#   JUDGE_SITES=A,B,C      judge apps to launch (default A,B,C). Leave empty for no judge apps
#   JUDGE_AE_DIR=…         folder containing judge_ae.py (default: judge_ae/ in this folder)
#   FL_SENSOR_FROM_CSE=0   offline: build pkl from the telemetry in AMR_DATA and train on it
#   FL_RESET_ON_START=0    do not delete the previous run's local models and caches

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"
cd "$HERE/site_app"

fl_load_env
fl_require_torch

FROM_CSE="${FL_SENSOR_FROM_CSE:-1}"
JUDGE_SITES="${JUDGE_SITES-A,B,C}"
[ "$FROM_CSE" = "0" ] && JUDGE_SITES=""

echo
echo "[1/5] Clean up previous run"
# Only stop existing judge_ae processes when this script launches the judge apps.
# (If JUDGE_SITES is empty, judge apps someone else launched separately are left alone)
if [ -n "$JUDGE_SITES" ]; then
  fl_free_ports "5001 5002 5003" "mn_ae_standard.py" "judge_ae.py"
else
  fl_free_ports "5001 5002 5003" "mn_ae_standard.py"
fi
if [ "${FL_RESET_ON_START:-1}" = "1" ]; then
  BASE="${FL_MODEL_BASE_DIR:-/tmp/fl_models}"
  rm -rf "$BASE/local" "$BASE/cache"
  mkdir -p "$BASE/local" "$BASE/cache"
  echo "  ✓ cleared local model/cache: $BASE"
fi

if [ "$FROM_CSE" = "0" ]; then
  echo
  echo "[2/5] Offline mode: prepare pkl (telemetry.json in AMR_DATA)"
  python3 prepare_data_amr.py
  echo
  echo "[3/5] Wait for robots — skipped in offline mode"
else
  fl_warn_notify_host
  echo
  echo "[2/5] Check MN-CSE resources"
  # Only our resources are checked (TRAINING-AE-i, ANALYSIS-AE-i, containers, ACPs).
  # Robot AEs are owned by Isaac, so missing ones do not count as failures.
  if ! python3 setup_resources_standard.py --scope mn --verify; then
    echo
    echo "  Resources are missing. Creating them first."
    python3 setup_resources_standard.py --scope mn
  fi
  echo
  echo "[3/5] Wait for Isaac Sim robots (at least ${FL_MIN_ROBOTS_PER_SITE:-1} per site)"
  if ! python3 setup_resources_standard.py --scope mn --wait-robots; then
    echo
    echo "  ⚠ Not all robots are ready. Continuing anyway."
    echo "    (late robots are included from that round on)"
  fi
fi

export FL_STANDALONE=0
export FL_LOG_DIR="$(fl_log_dir)"

echo
echo "[4/5] 3 site apps"
for s in A B C; do
  bash "$HERE/run_site_app.sh" "$s" &
  FL_PIDS+=("$!")
done

echo
echo "[5/5] Judge apps"
if [ -z "$JUDGE_SITES" ]; then
  echo "  not started (JUDGE_SITES is empty or offline mode)."
else
  for s in ${JUDGE_SITES//,/ }; do
    bash "$HERE/run_judge.sh" "$s" &
    FL_PIDS+=("$!")
  done
fi

sleep 2
echo
echo "============================================================"
echo "  All started. Logs: $FL_LOG_DIR/"
echo "  Ctrl+C to stop all"
echo "============================================================"
wait
