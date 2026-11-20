#!/bin/bash
# run_dashboard.sh — start the FL dashboard (http://<host>:7000)
#
#   bash run_dashboard.sh         (can be run from any directory)
#
# Reads from
#   Central IN-CSE    round state, per-site summary (loss, sample count), global model, per-round evaluation
#   Site MN-CSE       per-window anomaly scores (TRAINING-AE-i/cnt-window-scores)
#
# Site scores do not leave the site (IN-CSE), so they are read directly from the MN-CSE.
# So it runs on an admin host outside the sandbox that can reach both the IN-CSE and the
# site MN-CSEs. It can be started at any time.
#
# Prerequisite: .env.local in this folder or the repo root has FL_SANDBOX_HOST / FL_SANDBOX_NAME,
#   FL_IN_BASE_URL / FL_IN_CSE_NAME (+ FL_IN_USER / FL_IN_PASS).

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"
cd "$HERE"

fl_load_env
fl_require_torch

PORT="${FL_DASHBOARD_PORT:-7000}"
echo
echo "[1/2] 지난 실행 정리"
fl_free_ports "$PORT" "dashboard_server.py"

echo
echo "[2/2] 대시보드 -> http://localhost:${PORT}"
LOG_DIR="$(fl_log_dir)"
echo "  로그: $LOG_DIR/dashboard.log"
fl_start "$LOG_DIR/dashboard.log" python3 -u dashboard_server.py
wait
