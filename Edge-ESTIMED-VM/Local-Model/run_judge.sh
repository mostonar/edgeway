#!/bin/bash
# run_judge.sh — start one judge app (JUDGE-AE-n)
#
#   bash run_judge.sh A|B|C
#
# Subscribes to ANALYSIS-AE-n/cnt-verdict on that site's MN-CSE and decides whether it is a floor fault
# or a robot fault (judge_ae/judge_ae.py). The --cse and --iot addresses are built from .env.local by the
# same rules as the site app. Notifications arrive via pollingChannel, so
# no port needs to be opened.
#
# Start after the site app has created the MN-CSE resources (ANALYSIS-AE-n/cnt-verdict).
# JUDGE_AE_DIR changes where judge_ae.py lives (default: judge_ae/ in this folder).

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

# Build CSE addresses with the same rules as the site app (honoring overrides like FL_MN_BASE_URL_MNx).
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
