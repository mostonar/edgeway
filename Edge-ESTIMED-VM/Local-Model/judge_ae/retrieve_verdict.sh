#!/bin/bash
# Keep RETRIEVE-ing a container and print whatever newly appears (debug helper for the detection AE output).
#   CSE=<CSE base URL incl. CSE name> ./retrieve_verdict.sh             ANALYSIS-AE-3/cnt-verdict, every 1 s
#   CSE=... SITE=A ./retrieve_verdict.sh                                site A/B/C -> ANALYSIS-AE-1/2/3 (default C)
#   CSE=... ./retrieve_verdict.sh ANALYSIS-AE-3/cnt-verdict 0.5        target path, interval (s)
#   CSE=... ORIGIN=CJUDGE-AE-3 ./retrieve_verdict.sh                    retrieve as another originator (default CAdmin)
# Prints container state changes, and the position of each new CIN with "verdict": "ANOMALY" (yellow).
# Other verdicts (NORMAL etc.) are not printed. Ctrl+C to stop.

CSE=${CSE:?set CSE=<CSE base URL incl. CSE name>}
case ${SITE:-C} in A|a) N=1 ;; B|b) N=2 ;; C|c) N=3 ;; *) echo "SITE must be A, B or C"; exit 1 ;; esac
TARGET=${1:-ANALYSIS-AE-$N/cnt-verdict}
GAP=${2:-1}
ORIGIN=${ORIGIN:-CAdmin}

get() {  # get <path> -> "<body><TAB><http code>"
  curl -sk -m 5 -w '\t%{http_code}' -H "X-M2M-Origin: $ORIGIN" -H "X-M2M-RI: w$RANDOM" -H 'X-M2M-RVI: 3' \
       -H 'Accept: application/json' "$CSE/$1"
}

echo "Watching: $CSE/$TARGET  (originator $ORIGIN, every ${GAP} s)"
last_state=""; last_ri=""
while true; do
  r=$(get "$TARGET/la"); code=${r##*$'\t'}; body=${r%$'\t'*}
  case $code in
    200) state="CIN present" ;;
    404) # no latest CIN -> check whether the container itself exists
         c2=$(get "$TARGET"); c2=${c2##*$'\t'}
         [ "$c2" = 200 ] && state="container exists, no CIN yet" || state="container not found (404)" ;;
    000) state="cannot reach CSE" ;;
    *)   state="response $code" ;;
  esac
  [ "$state" != "$last_state" ] && { echo "$(date +%T) [state] $state"; last_state=$state; }
  if [ "$code" = 200 ]; then
    line=$(python3 -c '
import sys, json
c = json.loads(sys.argv[1])["m2m:cin"]
con = c.get("con")
try: con = json.loads(con) if isinstance(con, str) else con
except Exception: con = {}
p = (con or {}).get("pose") or {}
print(c.get("ri", ""), (con or {}).get("verdict", ""), p.get("x", "?"), p.get("y", "?"), sep="\t")' "$body" 2>/dev/null)
    IFS=$'\t' read -r ri verdict x y <<< "$line"
    if [ -n "$ri" ] && [ "$ri" != "$last_ri" ]; then
      [ "$verdict" = ANOMALY ] && printf '\033[33m%s ANOMALY detected at (%s, %s)\033[0m\n' "$(date +%T)" "$x" "$y"
      last_ri=$ri
    fi
  fi
  sleep "$GAP"
done
