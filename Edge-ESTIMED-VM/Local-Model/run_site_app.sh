#!/bin/bash
# run_site_app.sh — 사업장 앱 하나 기동 (TRAINING-AE-i + ANALYSIS-AE-i)
#
#   bash run_site_app.sh A        사업장 A (mn1, 포트 5001)
#   bash run_site_app.sh B        사업장 B (mn2, 포트 5002)
#   bash run_site_app.sh C        사업장 C (mn3, 포트 5003)
#
# 혼자 실행하면: 그 사업장의 지난 실행 정리 → MN-CSE 리소스 확인/생성
#   → 로봇 대기 → 사업장 앱.
# run_local_model.sh 가 부를 때는 정리·리소스·로봇 대기를 그쪽이 한 번에 하고
# (FL_STANDALONE=0), 여기서는 사업장 앱만 띄운다.
#
# 전제: .env.local 에 FL_SANDBOX_HOST / FL_SANDBOX_NAME, FL_IN_BASE_URL /
#   FL_IN_CSE_NAME, FL_NOTIFY_HOST. 자세한 것은 README.md.

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"

if ! INFO="$(fl_site "${1:-}")"; then
  echo "사용법: bash $0 A|B|C"
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
  echo "[사업장 ${SITE}] 지난 실행 정리"
  fl_free_ports "$PORT" "mn_ae_standard.py ${IDX} ${PORT}"
  if [ "${FL_RESET_ON_START:-1}" = "1" ]; then
    BASE="${FL_MODEL_BASE_DIR:-/tmp/fl_models}"
    rm -rf "$BASE/local/$NODE" "$BASE/cache/$NODE"
    echo "  ✓ 로컬 모델/캐시 비움: $BASE/{local,cache}/$NODE"
  fi

  if [ "$FROM_CSE" = "0" ]; then
    echo
    echo "[사업장 ${SITE}] 오프라인 모드: pkl 준비 (AMR_DATA 의 telemetry.json)"
    python3 prepare_data_amr.py
  else
    fl_warn_notify_host
    echo
    echo "[사업장 ${SITE}] MN-CSE 리소스 확인"
    if ! python3 setup_resources_standard.py --scope mn --nodes "$NODE" --verify; then
      echo
      echo "  리소스가 빠져 있다. 먼저 생성한다."
      python3 setup_resources_standard.py --scope mn --nodes "$NODE"
    fi
    echo
    echo "[사업장 ${SITE}] Isaac Sim 로봇 대기 (최소 ${FL_MIN_ROBOTS_PER_SITE:-1}대)"
    if ! python3 setup_resources_standard.py --scope mn --nodes "$NODE" --wait-robots; then
      echo "  ⚠ 로봇이 다 준비되지 않았다. 그래도 진행한다."
    fi
  fi
fi

LOG_DIR="$(fl_log_dir)"
echo
echo "[사업장 ${SITE}] TRAINING-AE-$((IDX + 1)) 기동 (포트 ${PORT})  로그: $LOG_DIR/${NODE}.log"
fl_start "$LOG_DIR/${NODE}.log" python3 -u mn_ae_standard.py "$IDX" "$PORT"
wait
