#!/bin/bash
# run_fedavg.sh — start the FL coordinator (IN-AE)
#
#   bash run_fedavg.sh            (can be run from any directory)
#
# Order: clean up previous run → check/create IN-CSE resources → IN-AE
#
# Prerequisites
#   - .env.local in this folder or the repo root has FL_IN_BASE_URL / FL_IN_CSE_NAME
#     (and FL_IN_USER / FL_IN_PASS if needed).
#
# Either this or the site apps (Local-Model) can be started first. Before the first round the IN-AE
# waits FL_COMMAND_SUBSCRIPTION_WAIT seconds (900 s by default in this script) for the
# cnt-fl-control subscriptions of the 3 site apps.
#
# Common settings
#   FL_GLOBAL_ROUNDS=10     number of rounds
#   FL_VERIFY_ACP=0         turn off the ACP check at startup (a diagnostic that connects directly to the site MN-CSEs)
#   FL_RESET_ON_START=0     do not delete the previous run's global model files (/tmp/fl_models/global)

set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/fl_env.sh"
cd "$HERE"

fl_load_env
fl_require_torch

export FL_COMMAND_SUBSCRIPTION_WAIT="${FL_COMMAND_SUBSCRIPTION_WAIT:-900}"

# The startup ACP check connects directly to the site MN-CSEs. If this host has no sandbox address
# there is nowhere to connect and it just keeps retrying, so turn it off. (The check is diagnostic and unrelated to training)
if [ -z "${FL_SANDBOX_HOST:-}" ] && [ -z "${FL_VERIFY_ACP:-}" ]; then
  export FL_VERIFY_ACP=0
  echo "  · FL_SANDBOX_HOST unset; disabling startup ACP check (FL_VERIFY_ACP)"
fi

echo
echo "[1/3] clean up previous run"
fl_free_ports "6000" "in_ae_standard.py"
if [ "${FL_RESET_ON_START:-1}" = "1" ]; then
  BASE="${FL_MODEL_BASE_DIR:-/tmp/fl_models}"
  # Global model files the IN-AE leaves every round. Clear the previous run's.
  rm -rf "$BASE/global"
  mkdir -p "$BASE/global"
  echo "  ✓ cleared global model files: $BASE/global"
fi

echo
echo "[2/3] check IN-CSE resources"
if ! python3 setup_resources_standard.py --scope in --verify; then
  echo
  echo "  Resources missing. Creating them first."
  python3 setup_resources_standard.py --scope in
fi

echo
echo "[3/3] IN-AE (coordinator)"
LOG_DIR="$(fl_log_dir)"
echo "  log: $LOG_DIR/in_ae.log"
echo "  waiting up to ${FL_COMMAND_SUBSCRIPTION_WAIT}s for site app subscriptions"
fl_start "$LOG_DIR/in_ae.log" python3 -u in_ae_standard.py
wait
