# fl_env.sh — shared functions sourced at the top of the run scripts in this folder.
# Not meant to be run directly. An identical copy lives in each of FL-FedAvg, FL-Dashboard and Local-Model
# (copied on purpose so each folder can be deployed on its own).
#
# All scripts work relative to their own folder only. Same result wherever they sit in the repo
# and whichever directory they run from. Outer folders are looked at only to find .env.local.

FL_HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export FL_HERE

# Read .env.local. Looks in this folder → one level up → two levels up, in that order.
# The repo root is one level up from FL-Dashboard and two levels up from Cloud/FL-FedAvg and
# Edge-ESTIMED-VM/Local-Model, so a single file at the root is found by all.
# Values already in the shell are not overwritten (env vars set in front win).
fl_load_env() {
  local f="" d
  for d in "$FL_HERE" "$FL_HERE/.." "$FL_HERE/../.."; do
    if [ -f "$d/.env.local" ]; then
      f="$(cd "$d" && pwd)/.env.local"
      break
    fi
  done
  if [ -z "$f" ]; then
    echo "  ⚠ No .env.local ($FL_HERE and two levels up). Addresses fall back to defaults."
    echo "    cp $FL_HERE/env.local.example $FL_HERE/.env.local  then fill it in"
    return 0
  fi
  local line name
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    line="${line#"${line%%[![:space:]]*}"}"      # Strip leading whitespace
    line="${line#export }"
    case "$line" in
      [A-Za-z_]*=*) ;;
      *) continue ;;                               # Comments, blank lines
    esac
    name="${line%%=*}"
    if [ -z "${!name+x}" ]; then
      eval "export $line"
    fi
  done < "$f"
  echo "  ✓ .env.local loaded ($f)"
}

fl_require_torch() {
  if ! python3 -c "import torch" 2>/dev/null; then
    echo
    echo "  ✗ current python3 has no torch: $(command -v python3)"
    echo "    activate the virtualenv and run again (pip install -r $FL_HERE/requirements.txt)"
    exit 1
  fi
}

# Clean up ports and processes.
#   fl_free_ports "port1 port2" "process pattern1" "process pattern2" ...
# Patterns are matched against the command line with pkill -f (spaces are fine).
fl_free_ports() {
  local ports="$1"; shift
  local killed=0 pat port pid
  for pat in "$@"; do
    pkill -TERM -f "$pat" 2>/dev/null && killed=1
  done
  [ "$killed" = "1" ] && sleep 2
  for pat in "$@"; do
    pkill -KILL -f "$pat" 2>/dev/null || true
  done
  for port in $ports; do
    pid="$(ss -ltnpH "sport = :${port}" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || true)"
    if [ -n "${pid:-}" ]; then
      echo "  · port ${port} is held by PID ${pid} -> stopping it"
      kill -TERM "$pid" 2>/dev/null || true
      sleep 1
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
  [ -n "$ports" ] && echo "  ✓ cleaned up: ports ${ports}"
  return 0
}

# Per-run log folder. Uses FL_LOG_DIR if set (passed down by the parent script).
fl_log_dir() {
  if [ -n "${FL_LOG_DIR:-}" ]; then
    mkdir -p "$FL_LOG_DIR"; echo "$FL_LOG_DIR"; return
  fi
  local d="$FL_HERE/logs/run_$(date +%Y%m%d_%H%M%S)"
  mkdir -p "$d"
  echo "$d"
}

# Remember launched processes and bring them down when the script ends or gets Ctrl+C / kill.
FL_PIDS=()
fl_cleanup() {
  if [ ${#FL_PIDS[@]} -gt 0 ]; then
    kill "${FL_PIDS[@]}" 2>/dev/null || true
    wait 2>/dev/null || true
  fi
}
trap fl_cleanup EXIT INT TERM

# Launch a command in the background and send its output to both the screen and a log file.
#   fl_start <logfile> <command...>
# Piping into tee makes $! the PID of tee, which leaves python running when stopping.
# So only the output goes to tee, and $! stays the PID of the command itself.
fl_start() {
  local log="$1"; shift
  "$@" > >(tee -a "$log") 2>&1 &
  FL_PIDS+=("$!")
}

# Site name (A/B/C, siteA, mn1, 1 …) → "A 0 mn1 5001"
fl_site() {
  local s="${1:-}"
  s="$(echo "$s" | tr '[:lower:]' '[:upper:]')"
  s="${s#SITE}"
  case "$s" in
    A|MN1|1) echo "A 0 mn1 5001" ;;
    B|MN2|2) echo "B 1 mn2 5002" ;;
    C|MN3|3) echo "C 2 mn3 5003" ;;
    *) return 1 ;;
  esac
}

# Whether the address the MN-CSE uses to notify the site app is usable
fl_warn_notify_host() {
  case "${FL_NOTIFY_HOST:-127.0.0.1}" in
    127.0.0.1|localhost)
      echo
      echo "  ⚠ FL_NOTIFY_HOST is ${FL_NOTIFY_HOST:-empty (127.0.0.1)}."
      echo "    MN-CSE runs inside the sandbox container, so with this address it cannot"
      echo "    notify the site apps (5001-5003). Put this host's IP in .env.local."
      ;;
  esac
}
