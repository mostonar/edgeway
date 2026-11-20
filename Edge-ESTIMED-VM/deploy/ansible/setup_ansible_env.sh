#!/usr/bin/env bash

set -euo pipefail

PLAYBOOK_DIR="$HOME/etsi-mec-sandbox/deploy/ansible"
SANDBOX_DIR="$HOME/etsi-mec-sandbox"
VENV_NAME="ansible-venv"
VENV_PATH="$PLAYBOOK_DIR/$VENV_NAME"
COLLECTION_REQ="$PLAYBOOK_DIR/collections/requirements.yml"
GITIGNORE_FILE="$SANDBOX_DIR/.gitignore"
GITIGNORE_ENTRY="deploy/ansible/$VENV_NAME/"

error() { echo "ERROR: $1" >&2; exit 1; }
command_exists() { command -v "$1" >/dev/null 2>&1; }

require_ubuntu() {
    [[ -f /etc/os-release ]] || error "Cannot detect OS."
    . /etc/os-release
    [[ "$ID" == "ubuntu" ]] || error "This script supports Ubuntu only."
}

confirm_install() {
    local pkg="$1"
    read -r -p "$pkg is missing. Install it now? [y/N]: " ans
    [[ "$ans" =~ ^[Yy]$ ]] || error "$pkg is required. Aborting."
    sudo apt-get update
    sudo apt-get install -y "$pkg"
}

ensure_package() {
    local cmd="$1" pkg="$2"
    command_exists "$cmd" || confirm_install "$pkg"
}

create_venv() {
    if [[ -d "$VENV_PATH" ]]; then
        echo "Virtual environment exists, skipping creation."
    else
        python3 -m venv "$VENV_PATH"
        echo "Virtual environment created at $VENV_PATH"
    fi
}

activate_venv() { source "$VENV_PATH/bin/activate"; }

install_python_packages() {
    pip install --upgrade pip
    pip install kubernetes ansible
}

install_ansible_collections() {
    [[ -f "$COLLECTION_REQ" ]] || error "Missing $COLLECTION_REQ"
    ansible-galaxy collection install -r "$COLLECTION_REQ" || true
}

update_gitignore() {
    [[ -f "$GITIGNORE_FILE" ]] || touch "$GITIGNORE_FILE"

    if ! grep -q "^deploy/ansible/$VENV_NAME" "$GITIGNORE_FILE"; then
        echo "deploy/ansible/$VENV_NAME/" >> "$GITIGNORE_FILE"
        echo "Added deploy/ansible/$VENV_NAME/ to .gitignore"
    else
        echo ".gitignore already contains entry for $VENV_NAME"
    fi
}

main() {
    require_ubuntu

    ensure_package python3 python3
    ensure_package python3 -m venv python3-venv
    ensure_package pip3 python3-pip

    create_venv
    activate_venv

    install_python_packages
    install_ansible_collections
    update_gitignore

    echo "Setup complete. Activate environment with:"
    echo "source $VENV_PATH/bin/activate"
}

main