#!/bin/bash
# run_dashboard.sh — FL 대시보드 기동 (http://<호스트>:7000)
#
#   bash run_dashboard.sh         (어느 디렉터리에서 실행해도 된다)
#
# 읽는 곳
#   중앙 IN-CSE    라운드 상태, 사업장별 요약(손실·샘플 수), 글로벌 모델, 라운드별 평가
#   사업장 MN-CSE  윈도우별 이상 점수 (TRAINING-AE-i/cnt-window-scores)
#
# 사업장 점수는 사업장 밖(IN-CSE)으로 나가지 않으므로 MN-CSE 에서 직접 읽는다.
# 그래서 샌드박스(사업장 MN-CSE)에 닿는 호스트에서 띄운다. 지금 배치에서는
# Edge VM 이다. 언제 띄워도 된다.
#
# 전제: 이 폴더나 저장소 루트의 .env.local 에 FL_SANDBOX_HOST / FL_SANDBOX_NAME,
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
