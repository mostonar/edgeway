#!/bin/bash

set -e # Exit immediately on errors

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

# Configure environment
export PATH=$PATH:/usr/local/go/bin:$HOME/gocode/bin
export GOOS=linux
IMAGE_NAME="meepctl"
BINDIR="../../bin/meepctl"

echo ""
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
printf "%b\n" "${CYAN}${BOLD}🚀 Installing Go Application: ${IMAGE_NAME}${NC}"
printf "%b\n" "${CYAN}${BOLD}==================================================${NC}"
echo ""

cd "$BASEDIR" || exit 1

# Clean build
printf "%b\n" "${BLUE}${BOLD}➤ Cleaning build artifacts${NC}"
go clean

echo ""
# Create vendor folder
printf "%b\n" "${BLUE}${BOLD}➤ Vendoring dependencies${NC}"
go mod vendor

echo ""
# Lint code
printf "%b\n" "${BLUE}${BOLD}➤ Linting codebase${NC}"
golangci-lint run

echo ""
# Build
printf "%b\n" "${BLUE}${BOLD}➤ Compiling binary${NC}"
go build -o "./$IMAGE_NAME" .

echo ""
# Copy to bin folder
printf "%b\n" "${BLUE}${BOLD}➤ Copying binary to ${BINDIR}${NC}"
mkdir -p "$BINDIR"
cp "./$IMAGE_NAME" "$BINDIR"

echo ""
# Install
printf "%b\n" "${BLUE}${BOLD}➤ Running go install${NC}"
go install

echo ""
# Configure sudoers for meepctl certificate trust operations (when HTTPS)
printf "%b\n" "${BLUE}${BOLD}➤ Configuring sudoers NOPASSWD for meepctl certificate operations${NC}"
TARGET_USER="${SUDO_USER:-$USER}"
SUDOERS_RULE="${TARGET_USER} ALL=(ALL:ALL) NOPASSWD: /usr/bin/cp, /bin/cp, /usr/sbin/update-ca-certificates, /usr/bin/update-ca-certificates, /usr/bin/systemctl restart containerd, /usr/bin/systemctl restart docker, /bin/systemctl restart containerd, /bin/systemctl restart docker, /usr/bin/mkdir, /bin/mkdir, /usr/bin/sh, /bin/sh, /usr/bin/sed, /bin/sed"
if [ -n "$SUDO_PASSWORD" ]; then
    echo "$SUDO_PASSWORD" | sudo -S sh -c "echo '$SUDOERS_RULE' > /etc/sudoers.d/meepctl && chmod 0440 /etc/sudoers.d/meepctl" 2>/dev/null || echo "⚠️  WARNING: Failed to create /etc/sudoers.d/meepctl (sudo required)"
elif sudo -n true 2>/dev/null; then
    sudo sh -c "echo '$SUDOERS_RULE' > /etc/sudoers.d/meepctl && chmod 0440 /etc/sudoers.d/meepctl" 2>/dev/null || echo "⚠️  WARNING: Failed to create /etc/sudoers.d/meepctl (sudo required)"
else
    sudo sh -c "echo '$SUDOERS_RULE' > /etc/sudoers.d/meepctl && chmod 0440 /etc/sudoers.d/meepctl" || echo "⚠️  WARNING: Failed to create /etc/sudoers.d/meepctl (sudo required)"
fi

echo ""
printf "%b\n" "${GREEN}${BOLD}✨ ${IMAGE_NAME} installation completed successfully!${NC}"
echo ""
