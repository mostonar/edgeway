#!/bin/sh

set -e # Exit immediately if a command exits with a non-zero status

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Automatically configure git to use the tracked hooks directory
git config core.hooksPath .githooks

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

MEEPDIR="$HOME/etsi-mec-sandbox"
CHECKSUM_FILE="$BASEDIR/.build_checksum"

calculate_package_checksum() {
    SRC_DIR="$1"
    find "$SRC_DIR" -type f \
        -not -path '*/node_modules/*' \
        -not -path '*/dist/*' \
        -not -path '*/.git/*' \
        2>/dev/null | sort | xargs md5sum 2>/dev/null | md5sum | cut -d' ' -f1
}

install_package() {
    PACKAGE_NAME="$1"
    SRC_DIR="$MEEPDIR/js-packages/$PACKAGE_NAME"
    DEST_DIR="$BASEDIR/js-packages/$PACKAGE_NAME"
    PKG_CHECKSUM_FILE="$DEST_DIR/.install_checksum"

    if [ -d "$DEST_DIR/node_modules" ] && [ -f "$PKG_CHECKSUM_FILE" ]; then
        SRC_CHECKSUM=$(calculate_package_checksum "$SRC_DIR")
        SAVED_CHECKSUM=$(cat "$PKG_CHECKSUM_FILE" 2>/dev/null)
        if [ "$SRC_CHECKSUM" = "$SAVED_CHECKSUM" ]; then
            echo ""
            printf "%b\n" "${GREEN}✔ Package ${BOLD}${PACKAGE_NAME}${NC}${GREEN} is up-to-date. Skipping installation.${NC}"
            return 0
        fi
    fi

    echo ""
    printf "%b\n" "${YELLOW}⧗ Installing ${BOLD}${PACKAGE_NAME}${NC}"
    rm -rf "$DEST_DIR"
    mkdir -p "$DEST_DIR"
    cp -r "$SRC_DIR" "$BASEDIR/js-packages"
    
    # Calculate checksum of the source files
    SRC_CHECKSUM=$(calculate_package_checksum "$SRC_DIR")
    
    cd "$DEST_DIR" || exit 1
    npm ci
    
    echo "$SRC_CHECKSUM" > "$PKG_CHECKSUM_FILE"
}

calculate_checksum() {
    find "$BASEDIR/js-apps/frontend" "$MEEPDIR/js-packages" -type f \
        -not -path '*/node_modules/*' \
        -not -path '*/dist/*' \
        -not -path '*/bin/*' \
        -not -path '*/.git/*' \
        2>/dev/null | sort | xargs md5sum 2>/dev/null | md5sum | cut -d' ' -f1
}

# Check if build can be skipped
if [ -d "$BASEDIR/bin/frontend" ] && [ -f "$CHECKSUM_FILE" ]; then
    CURRENT_CHECKSUM=$(calculate_checksum)
    SAVED_CHECKSUM=$(cat "$CHECKSUM_FILE" 2>/dev/null)
    if [ "$CURRENT_CHECKSUM" = "$SAVED_CHECKSUM" ]; then
        echo ""
        printf "%b\n" "${GREEN}${BOLD}==================================================${NC}"
        printf "%b\n" "${GREEN}${BOLD}✔ MEC Sandbox Frontend: No changes detected. Skipping build.${NC}"
        printf "%b\n" "${GREEN}${BOLD}==================================================${NC}"
        echo ""
        exit 0
    fi
fi

echo ""
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
printf "%b\n" "${CYAN}${BOLD}🚀 Building MEC Sandbox Frontend${NC}"
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
echo ""
cd "$BASEDIR" || exit 1
rm -rf bin/
mkdir -p bin/frontend

printf "%b\n" "${BLUE}${BOLD}➤ Linting Frontend${NC}"
cd "$BASEDIR/js-apps/frontend" || exit 1
npm install
npx eslint src

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Installing client packages${NC}"
mkdir -p "$BASEDIR/js-packages"
install_package meep-auth-svc-client
install_package meep-gis-engine-client
install_package meep-metrics-engine-client
install_package meep-platform-ctrl-client
install_package meep-sandbox-ctrl-client
install_package meep-mon-engine-client

echo ""
printf "%b\n" "${BLUE}${BOLD}➤ Compiling Frontend (Webpack)${NC}"
cd "$BASEDIR/js-apps/frontend" || exit 1
rm -rf dist
npm run build -- --output-path="$BASEDIR/bin/frontend"

# Save the checksum after a successful or attempted build
calculate_checksum > "$CHECKSUM_FILE"

echo ""
printf "%b\n" "${GREEN}${BOLD}✨ MEC Sandbox build completed successfully!${NC}"
echo ""
