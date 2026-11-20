#!/bin/bash
# run_judge.sh — 판정 앱 하나 기동 (JUDGE-AE-n)
#
#   bash run_judge.sh A|B|C
#
# 그 사업장 MN-CSE 의 ANALYSIS-AE-n/cnt-verdict 를 구독해 바닥 고장인지
# 로봇 고장인지 가른다(judge_ae/judge_ae.py). --cse 와 --iot 주소는 사업장 앱과
# 같은 규칙으로 .env.local 에서 만든다. 알림은 pollingChannel 로 받으므로
# 포트를 열 필요가 없다.
#
# 사업장 앱이 MN-CSE 리소스(ANALYSIS-AE-n/cnt-verdict)를 만든 뒤에 띄운다.
# JUDGE_AE_DIR 로 judge_ae.py 위치를 바꿀 수 있다(기본: 이 폴더의 judge_ae/).

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"

if ! INFO="$(fl_site "${1:-}")"; then
  echo "사용법: bash $0 A|B|C"
  exit 2
fi
read -r SITE IDX NODE PORT <<< "$INFO"
JUDGE_AE_DIR="${JUDGE_AE_DIR:-$HERE/judge_ae}"

if [ "${FL_STANDALONE:-1}" = "1" ]; then
  fl_load_env
  fl_free_ports "" "judge_ae.py --site ${SITE} "
fi

if [ ! -f "$JUDGE_AE_DIR/judge_ae.py" ]; then
  echo "  ✗ $JUDGE_AE_DIR/judge_ae.py 가 없다. JUDGE_AE_DIR 로 지정할 것."
  exit 1
fi

# 사업장 앱과 같은 규칙으로 CSE 주소를 만든다 (FL_MN_BASE_URL_MNx 등 덮어쓰기 존중).
cd "$HERE/site_app"
CSE_URL=""; IOT_URL=""
read -r CSE_URL IOT_URL < <(python3 - "$NODE" <<'PY'
import sys
import config, mec_iot
node = sys.argv[1]
base, cse = config.mn_endpoint(node)
print(f"{base}/{cse}", f"{mec_iot.iot_base(config.MEP_OF_NODE[node])}/registered_iot_platforms")
PY
) || true
if [ -z "$CSE_URL" ]; then
  echo "  ✗ 사업장 ${SITE} 의 CSE 주소를 못 만들었다. .env.local 의 FL_SANDBOX_HOST / FL_SANDBOX_NAME 을 확인할 것."
  exit 1
fi

INSECURE=()
case "${FL_TLS_VERIFY:-1}" in 0|false|False) INSECURE=(--insecure) ;; esac

LOG_DIR="$(fl_log_dir)"
echo
echo "[판정 ${SITE}] JUDGE-AE-$((IDX + 1)) -> $CSE_URL  로그: $LOG_DIR/judge_${SITE}.log"
fl_start "$LOG_DIR/judge_${SITE}.log" \
  python3 -u "$JUDGE_AE_DIR/judge_ae.py" --site "$SITE" --cse "$CSE_URL" --iot "$IOT_URL" "${INSECURE[@]}"
wait
