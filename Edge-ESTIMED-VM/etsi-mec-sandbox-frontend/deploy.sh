#!/bin/sh

set -e # Exit on error

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

MEEPDIR="$HOME/etsi-mec-sandbox"
CONFIGDIR="$MEEPDIR/config"
DASHBOARDDIR="$MEEPDIR/charts/observability/grafana/dashboards"
APIDIR="$MEEPDIR/config/api"
GRIDDIR="$MEEPDIR/go-packages/meep-vis-traffic-mgr"
FRONTENDDIR="$HOME/.meep/user/frontend"
VALUESDIR="$HOME/.meep/user/values"

echo ""
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
printf "%b\n" "${CYAN}${BOLD}🚀 Deploying MEC Sandbox Frontend${NC}"
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
echo ""

printf "%b\n" "${BLUE}${BOLD}➤ Updating AdvantEDGE configuration${NC}"
printf "  MEEP Directory:   %s\n" "$MEEPDIR"
printf "  Config Directory: %s\n" "$CONFIGDIR"

cd "$BASEDIR" || exit 1
cp config/.meepctl-repocfg.yaml "$MEEPDIR"
cp config/permissions.yaml "$CONFIGDIR"
cp config/secrets.yaml "$CONFIGDIR"
cp config/objstore-influx.cfg "$CONFIGDIR"
cp config/objstore-thanos.yaml "$CONFIGDIR"
cp config/objstore-thanos-archive.yaml "$CONFIGDIR"

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Copying Swagger API files${NC}"
printf "  Target: %s\n" "$APIDIR"
mkdir -p "$APIDIR"
cp -r config/api/* "$APIDIR"

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Copying Grafana dashboards${NC}"
printf "  Target: %s\n" "$DASHBOARDDIR"
mkdir -p "$DASHBOARDDIR"
cp -r config/dashboards/* "$DASHBOARDDIR"

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Copying Grid Map file${NC}"
printf "  Target: %s\n" "$GRIDDIR"
mkdir -p "$GRIDDIR"
cp -r config/grid_map/* "$GRIDDIR"

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Deploying frontend binaries${NC}"
printf "  Target: %s\n" "$FRONTENDDIR"
mkdir -p "$FRONTENDDIR"
rm -rf "$FRONTENDDIR/"* 2> /dev/null || true
cp -r bin/frontend/* "$FRONTENDDIR"

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Updating configuration user values${NC}"
printf "  Target: %s\n" "$VALUESDIR"
mkdir -p "$VALUESDIR"
rm -rf "${VALUESDIR}-bak" 2> /dev/null || true
mkdir -p "${VALUESDIR}-bak"
cp -r "$VALUESDIR/"* "${VALUESDIR}-bak" 2>/dev/null || true
cp -r config/values/* "$VALUESDIR"

echo ""
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
printf "%b\n" "${CYAN}${BOLD}➤ Updating demo4-UE entrypoint.sh script${NC}"
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
echo ""

sed --in-place 's/advantedge/sandbox/' "$MEEPDIR/examples/demo4-ue/src/demo-server/entrypoint.sh"

echo ""
printf "%b\n" "${GREEN}${BOLD}✨ MEC Sandbox deployment completed successfully!${NC}"
echo ""
