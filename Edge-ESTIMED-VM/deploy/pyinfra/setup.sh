#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -e

# ==========================================
# Colors for output
# ==========================================
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# ==========================================
# Logging helpers
# ==========================================
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

echo -e "${BLUE}==========================================${NC}"
echo -e "${BLUE}    Pyinfra MEC Sandbox Setup Script      ${NC}"
echo -e "${BLUE}==========================================${NC}"
echo ""

# ==========================================
# 1. Check Python 3
# ==========================================
if ! command -v python3 >/dev/null 2>&1; then
    log_info "Python3 not found. Installing Python3..."
    sudo apt-get update
    sudo apt-get install -y python3
else
    log_success "Python3 is already installed."
fi

# ==========================================
# 2. Check Pip and Venv Packages
# ==========================================
# We check via dpkg to ensure the system packages for pip and venv are present on Ubuntu/Debian
MISSING_PKGS=""
if ! dpkg -s python3-pip >/dev/null 2>&1; then
    MISSING_PKGS="python3-pip"
fi

if ! dpkg -s python3-venv >/dev/null 2>&1; then
    MISSING_PKGS="$MISSING_PKGS python3-venv"
fi

if [ -n "$MISSING_PKGS" ]; then
    log_info "Missing python packages. Installing: $MISSING_PKGS"
    sudo apt-get update
    sudo apt-get install -y $MISSING_PKGS
else
    log_success "Python3 pip and venv packages are already installed."
fi

# ==========================================
# 3. Create Virtual Environment
# ==========================================
VENV_DIR="pyinfra-venv"
if [ ! -d "$VENV_DIR" ]; then
    log_info "Creating virtual environment in './$VENV_DIR'..."
    python3 -m venv "$VENV_DIR"
    log_success "Virtual environment created."
else
    log_success "Virtual environment '$VENV_DIR' already exists."
fi

# ==========================================
# 4. Install Pyinfra
# ==========================================
log_info "Activating virtual environment and installing pyinfra..."
source "$VENV_DIR/bin/activate"

# Upgrade pip quietly
pip install --upgrade pip >/dev/null 2>&1

# Install pyinfra if not already installed
if ! command -v pyinfra >/dev/null 2>&1; then
    log_info "Installing pyinfra..."
    pip install pyinfra==3.10.0 python-dotenv ruamel.yaml PyYAML
    log_success "pyinfra installed successfully."
else
    log_success "pyinfra is already installed in the virtual environment."
fi


# ==========================================
# 5. Setup Environment Variables (.env)
# ==========================================
if [ ! -f ".env" ]; then
    if [ -f ".env.example" ]; then
        log_info "Creating .env from .env.example..."
        cp .env.example .env
        echo ""
        # log_warn "A new .env file has been created from the template."
        # log_warn "You MUST update the .env file with your actual secrets, OAuth keys, and IP addresses."
        # log_error "The deployment WILL FAIL if the .env file is not properly configured!"
        echo ""
        # log_info "Please edit the .env file, then run the deployment:"
        # echo -e "  ${GREEN}nano .env${NC}"
        # echo -e "  ${GREEN}source $VENV_DIR/bin/activate${NC}"
        # echo -e "  ${GREEN}pyinfra inventory.py deploy.py${NC}"
        # exit 1
    else
        log_error ".env.example not found! Cannot create .env file."
        exit 1
    fi
else
    log_success ".env file already exists."
fi

# ==========================================
# Success Output
# ==========================================
echo ""
echo -e "${GREEN}========================================================================${NC}"
echo -e "${GREEN}   [SUCCESS] PyInfra Environment & Setup Complete!                      ${NC}"
echo -e "${GREEN}========================================================================${NC}"
echo ""
echo -e "${YELLOW}  [ACTION REQUIRED] Verify & Update Your Configuration (.env):${NC}"
echo -e "    Ensure ${BLUE}.env${NC} is updated with your target hosts and OAuth keys:"
echo -e "      - ${BLUE}K8S_MASTERS${NC}: e.g. \"localhost\" or \"ubuntu@192.168.1.10\""
echo -e "      - ${BLUE}MEC_HOST_ADDRESS${NC}: e.g. \"127.0.0.1\" or \"mec.example.com\""
echo -e "      - ${BLUE}OAuth Secrets${NC}: GitHub / GitLab Client ID and Secret"
echo ""
echo -e "${BLUE}  [NEXT STEPS] To execute the deployment, run the following commands:${NC}"
echo -e "    1. Activate the virtual environment:"
echo -e "       ${GREEN}source $VENV_DIR/bin/activate${NC}"
echo -e ""
echo -e "    2. Launch the automated deployment:"
echo -e "       ${GREEN}pyinfra inventory.py deploy.py -y${NC}"
echo -e "${GREEN}========================================================================${NC}"
echo ""
