#!/bin/bash
# run_fedavg.sh — FL 코디네이터(IN-AE) 기동
#
#   bash run_fedavg.sh            (어느 디렉터리에서 실행해도 된다)
#
# 순서: 지난 실행 정리 → IN-CSE 리소스 확인/생성 → IN-AE
#
# 전제
#   - 이 폴더나 저장소 루트의 .env.local 에 FL_IN_BASE_URL / FL_IN_CSE_NAME
#     (필요하면 FL_IN_USER / FL_IN_PASS) 가 있다.
#
# 사업장 앱(Local-Model)과 어느 쪽을 먼저 켜도 된다. IN-AE 는 첫 라운드 전에
# 사업장 앱 3개의 cnt-fl-control 구독을 FL_COMMAND_SUBSCRIPTION_WAIT 초
# (이 스크립트 기본 900초) 기다린다.
#
# 자주 쓰는 설정
#   FL_GLOBAL_ROUNDS=10     라운드 수
#   FL_VERIFY_ACP=0         시작 때 ACP 점검을 끈다(사업장 MN-CSE 에 직접 붙는 진단)
#   FL_RESET_ON_START=0     지난 실행의 글로벌 모델 파일(/tmp/fl_models/global)을 지우지 않는다

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"
cd "$HERE"

fl_load_env
fl_require_torch

export FL_COMMAND_SUBSCRIPTION_WAIT="${FL_COMMAND_SUBSCRIPTION_WAIT:-900}"

# 시작 때 ACP 점검은 사업장 MN-CSE 에 직접 붙는다. 이 호스트에 샌드박스 주소가
# 없으면 닿을 곳이 없어 재시도만 반복하므로 끈다. (점검은 진단용이라 학습과 무관)
if [ -z "${FL_SANDBOX_HOST:-}" ] && [ -z "${FL_VERIFY_ACP:-}" ]; then
  export FL_VERIFY_ACP=0
  echo "  · FL_SANDBOX_HOST 가 없어 시작 때 ACP 점검(FL_VERIFY_ACP)을 끈다"
fi

echo
echo "[1/3] 지난 실행 정리"
fl_free_ports "6000" "in_ae_standard.py"
if [ "${FL_RESET_ON_START:-1}" = "1" ]; then
  BASE="${FL_MODEL_BASE_DIR:-/tmp/fl_models}"
  # IN-AE 가 라운드마다 남기는 글로벌 모델 파일. 지난 실행 것을 비운다.
  rm -rf "$BASE/global"
  mkdir -p "$BASE/global"
  echo "  ✓ 글로벌 모델 파일 비움: $BASE/global"
fi

echo
echo "[2/3] IN-CSE 리소스 확인"
if ! python3 setup_resources_standard.py --scope in --verify; then
  echo
  echo "  리소스가 빠져 있다. 먼저 생성한다."
  python3 setup_resources_standard.py --scope in
fi

echo
echo "[3/3] IN-AE (코디네이터)"
LOG_DIR="$(fl_log_dir)"
echo "  로그: $LOG_DIR/in_ae.log"
echo "  사업장 앱 구독 대기 최대 ${FL_COMMAND_SUBSCRIPTION_WAIT}초"
fl_start "$LOG_DIR/in_ae.log" python3 -u in_ae_standard.py
wait
