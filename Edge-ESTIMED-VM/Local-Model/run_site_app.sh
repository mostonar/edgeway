#!/bin/bash
# run_site_app.sh — start one site app (TRAINING-AE-i + ANALYSIS-AE-i)
#
#   bash run_site_app.sh A        site A (mn1, port 5001)
#   bash run_site_app.sh B        site B (mn2, port 5002)
#   bash run_site_app.sh C        site C (mn3, port 5003)
#
# When run on its own: clean up that site's previous run → check/create MN-CSE resources
#   → wait for robots → site app.
# When called by run_local_model.sh, that script does cleanup, resources and waiting for robots once
# (FL_STANDALONE=0), and here only the site app is started.
#
# Prerequisites: .env.local has FL_SANDBOX_HOST / FL_SANDBOX_NAME, FL_IN_BASE_URL /
#   FL_IN_CSE_NAME, FL_NOTIFY_HOST. See README.md for details.

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"

if ! INFO="$(fl_site "${1:-}")"; then
  echo "Usage: bash $0 A|B|C"
  exit 2
fi
read -r SITE IDX NODE PORT <<< "$INFO"
STANDALONE="${FL_STANDALONE:-1}"
FROM_CSE="${FL_SENSOR_FROM_CSE:-1}"

cd "$HERE/site_app"

if [ "$STANDALONE" = "1" ]; then
  fl_load_env
  fl_require_torch
  echo
  echo "[site ${SITE}] Clean up previous run"
  fl_free_ports "$PORT" "mn_ae_standard.py ${IDX} ${PORT}"
  if [ "${FL_RESET_ON_START:-1}" = "1" ]; then
    BASE="${FL_MODEL_BASE_DIR:-/tmp/fl_models}"
    rm -rf "$BASE/local/$NODE" "$BASE/cache/$NODE"
    echo "  ✓ cleared local model/cache: $BASE/{local,cache}/$NODE"
  fi

  if [ "$FROM_CSE" = "0" ]; then
    echo
    echo "[site ${SITE}] Offline mode: prepare pkl (telemetry.json in AMR_DATA)"
    python3 prepare_data_amr.py
  else
    fl_warn_notify_host
    echo
    echo "[site ${SITE}] Check MN-CSE resources"
    if ! python3 setup_resources_standard.py --scope mn --nodes "$NODE" --verify; then
      echo
      echo "  Resources are missing. Creating them first."
      python3 setup_resources_standard.py --scope mn --nodes "$NODE"
    fi
    echo
    echo "[site ${SITE}] Wait for Isaac Sim robots (at least ${FL_MIN_ROBOTS_PER_SITE:-1})"
    if ! python3 setup_resources_standard.py --scope mn --nodes "$NODE" --wait-robots; then
      echo "  ⚠ Not all robots are ready. Continuing anyway."
    fi
  fi
fi

LOG_DIR="$(fl_log_dir)"
echo
echo "[site ${SITE}] TRAINING-AE-$((IDX + 1)) started (port ${PORT})  log: $LOG_DIR/${NODE}.log"
fl_start "$LOG_DIR/${NODE}.log" python3 -u mn_ae_standard.py "$IDX" "$PORT"
wait
