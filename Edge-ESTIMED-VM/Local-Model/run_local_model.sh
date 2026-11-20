#!/bin/bash
# run_local_model.sh — 사업장 앱 3개 + 판정 앱을 한 번에 기동
#
#   bash run_local_model.sh       (어느 디렉터리에서 실행해도 된다)
#
# 하나씩 띄우려면 run_site_app.sh A|B|C, run_judge.sh A|B|C 를 쓴다.
# 이 스크립트는 공통 준비(정리, MN-CSE 리소스, 로봇 대기)를 한 번만 하고
# 그 둘을 FL_STANDALONE=0 으로 부른다.
#
# 순서
#   지난 실행 정리 → MN-CSE 리소스 확인/생성 → Isaac Sim 로봇 대기
#   → 사업장 앱 3개 → 판정 앱 (JUDGE_SITES)
#
# 전제: .env.local 에 FL_SANDBOX_HOST / FL_SANDBOX_NAME, FL_IN_BASE_URL /
#   FL_IN_CSE_NAME, FL_NOTIFY_HOST. 자세한 것은 README.md.
#
# 자주 쓰는 설정
#   JUDGE_SITES=A,B,C      띄울 판정 앱 (기본 A,B,C). 비우면 판정 앱 없이
#   JUDGE_AE_DIR=…         judge_ae.py 가 있는 폴더 (기본: 이 폴더의 judge_ae/)
#   FL_SENSOR_FROM_CSE=0   오프라인: AMR_DATA 의 telemetry 로 pkl 을 만들어 학습
#   FL_RESET_ON_START=0    지난 실행의 로컬 모델·캐시를 지우지 않는다

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
# 판정 앱을 이 스크립트가 띄울 때만 기존 judge_ae 를 내린다.
# (JUDGE_SITES 를 비우면 다른 사람이 따로 띄운 판정 앱은 건드리지 않는다)
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
  # 우리 리소스(TRAINING-AE-i, ANALYSIS-AE-i, 컨테이너, ACP)만 본다.
  # 로봇 AE 는 Isaac 소유라 없어도 실패로 치지 않는다.
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
