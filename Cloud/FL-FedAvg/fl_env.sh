# fl_env.sh — 이 폴더의 실행 스크립트들이 맨 앞에서 source 하는 공용 함수.
# 직접 실행하는 파일이 아니다. 같은 내용이 FL-FedAvg, FL-Dashboard, Local-Model
# 세 폴더에 하나씩 있다(폴더마다 따로 배포할 수 있게 일부러 복사해 둠).
#
# 스크립트는 모두 자기 폴더 기준으로만 움직인다. 저장소 어디에 두든, 어느
# 디렉터리에서 실행하든 같다. 바깥 폴더는 .env.local 을 찾을 때만 본다.

FL_HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export FL_HERE

# .env.local 을 읽는다. 이 폴더 → 한 단계 위 → 두 단계 위 순서로 찾는다.
# 저장소 루트가 FL-Dashboard 에서는 한 단계 위, Cloud/FL-FedAvg 와
# Edge-ESTIMED-VM/Local-Model 에서는 두 단계 위라서, 루트에 하나만 두면 다 찾는다.
# 이미 셸에 있는 값은 덮어쓰지 않는다(앞에 붙인 환경변수가 이긴다).
fl_load_env() {
  local f="" d
  for d in "$FL_HERE" "$FL_HERE/.." "$FL_HERE/../.."; do
    if [ -f "$d/.env.local" ]; then
      f="$(cd "$d" && pwd)/.env.local"
      break
    fi
  done
  if [ -z "$f" ]; then
    echo "  ⚠ .env.local 이 없다 ($FL_HERE 와 그 위 두 단계). 주소가 기본값으로 간다."
    echo "    cp $FL_HERE/env.local.example $FL_HERE/.env.local  한 뒤 채울 것"
    return 0
  fi
  local line name
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    line="${line#"${line%%[![:space:]]*}"}"      # 앞 공백 제거
    line="${line#export }"
    case "$line" in
      [A-Za-z_]*=*) ;;
      *) continue ;;                               # 주석, 빈 줄
    esac
    name="${line%%=*}"
    if [ -z "${!name+x}" ]; then
      eval "export $line"
    fi
  done < "$f"
  echo "  ✓ .env.local 읽음 ($f)"
}

fl_require_torch() {
  if ! python3 -c "import torch" 2>/dev/null; then
    echo
    echo "  ✗ 지금 python3 에 torch 가 없다: $(command -v python3)"
    echo "    가상환경을 켜고 다시 실행할 것 (pip install -r $FL_HERE/requirements.txt)"
    exit 1
  fi
}

# 포트와 프로세스 정리.
#   fl_free_ports "포트1 포트2" "프로세스 패턴1" "프로세스 패턴2" ...
# 패턴은 pkill -f 로 명령줄과 맞춘다(공백을 넣어도 된다).
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
      echo "  · 포트 ${port} 를 PID ${pid} 가 잡고 있다 -> 내린다"
      kill -TERM "$pid" 2>/dev/null || true
      sleep 1
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
  [ -n "$ports" ] && echo "  ✓ 정리: 포트 ${ports}"
  return 0
}

# 실행별 로그 폴더. FL_LOG_DIR 이 있으면 그것을 쓴다(상위 스크립트가 넘겨줌).
fl_log_dir() {
  if [ -n "${FL_LOG_DIR:-}" ]; then
    mkdir -p "$FL_LOG_DIR"; echo "$FL_LOG_DIR"; return
  fi
  local d="$FL_HERE/logs/run_$(date +%Y%m%d_%H%M%S)"
  mkdir -p "$d"
  echo "$d"
}

# 띄운 프로세스를 기억했다가 스크립트가 끝나거나 Ctrl+C / kill 되면 같이 내린다.
FL_PIDS=()
fl_cleanup() {
  if [ ${#FL_PIDS[@]} -gt 0 ]; then
    kill "${FL_PIDS[@]}" 2>/dev/null || true
    wait 2>/dev/null || true
  fi
}
trap fl_cleanup EXIT INT TERM

# 명령을 백그라운드로 띄우고 출력을 화면과 로그 파일에 같이 남긴다.
#   fl_start <로그파일> <명령...>
# tee 를 파이프로 붙이면 $! 가 tee 의 PID 가 되어 끌 때 파이썬이 남는다.
# 그래서 출력만 tee 로 보내고 $! 는 명령 자신의 PID 가 되게 한다.
fl_start() {
  local log="$1"; shift
  "$@" > >(tee -a "$log") 2>&1 &
  FL_PIDS+=("$!")
}

# 사업장 이름(A/B/C, siteA, mn1, 1 …) → "A 0 mn1 5001"
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

# MN-CSE 가 사업장 앱에 알림을 보낼 주소가 쓸 만한지
fl_warn_notify_host() {
  case "${FL_NOTIFY_HOST:-127.0.0.1}" in
    127.0.0.1|localhost)
      echo
      echo "  ⚠ FL_NOTIFY_HOST 가 ${FL_NOTIFY_HOST:-비어 있음(127.0.0.1)} 이다."
      echo "    MN-CSE 는 샌드박스 컨테이너 안이라 이 주소로는 사업장 앱(5001-5003)에"
      echo "    알림을 못 보낸다. .env.local 에 이 호스트의 IP 를 넣을 것."
      ;;
  esac
}
