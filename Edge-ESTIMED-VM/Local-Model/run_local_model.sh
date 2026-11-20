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
echo "[1/5] 지난 실행 정리"
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
  echo "  ✓ 로컬 모델/캐시 비움: $BASE"
fi

if [ "$FROM_CSE" = "0" ]; then
  echo
  echo "[2/5] 오프라인 모드: pkl 준비 (AMR_DATA 의 telemetry.json)"
  python3 prepare_data_amr.py
  echo
  echo "[3/5] 로봇 대기 — 오프라인 모드라 건너뛴다"
else
  fl_warn_notify_host
  echo
  echo "[2/5] MN-CSE 리소스 확인"
  # Only our resources are checked (TRAINING-AE-i, ANALYSIS-AE-i, containers, ACPs).
  # Robot AEs are owned by Isaac, so missing ones do not count as failures.
  if ! python3 setup_resources_standard.py --scope mn --verify; then
    echo
    echo "  리소스가 빠져 있다. 먼저 생성한다."
    python3 setup_resources_standard.py --scope mn
  fi
  echo
  echo "[3/5] Isaac Sim 로봇 대기 (사업장당 최소 ${FL_MIN_ROBOTS_PER_SITE:-1}대)"
  if ! python3 setup_resources_standard.py --scope mn --wait-robots; then
    echo
    echo "  ⚠ 로봇이 다 준비되지 않았다. 그래도 진행한다."
    echo "    (뒤늦게 올라온 로봇은 그 라운드부터 포함된다)"
  fi
fi

export FL_STANDALONE=0
export FL_LOG_DIR="$(fl_log_dir)"

echo
echo "[4/5] 사업장 앱 3개"
for s in A B C; do
  bash "$HERE/run_site_app.sh" "$s" &
  FL_PIDS+=("$!")
done

echo
echo "[5/5] 판정 앱"
if [ -z "$JUDGE_SITES" ]; then
  echo "  띄우지 않는다 (JUDGE_SITES 가 비었거나 오프라인 모드)."
else
  for s in ${JUDGE_SITES//,/ }; do
    bash "$HERE/run_judge.sh" "$s" &
    FL_PIDS+=("$!")
  done
fi

sleep 2
echo
echo "============================================================"
echo "  전부 기동됨. 로그: $FL_LOG_DIR/"
echo "  Ctrl+C 로 전체 종료"
echo "============================================================"
wait
