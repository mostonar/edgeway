#!/bin/bash

set -e
set -x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

ENV_FILE="${1:-$BASEDIR/config.env}"

if [ ! -f "$ENV_FILE" ]; then
    echo "Env file not found: $ENV_FILE"
    echo "Usage: $0 [path/to/env-file]"
    exit 1
fi

# Read MEEP_IPE_PORT from the env file, default to 80 if not set/empty
PORT=$(grep -E '^MEEP_IPE_PORT=' "$ENV_FILE" | cut -d '=' -f2-)
PORT="${PORT:-80}"

docker run --rm -it \
    --env-file "$ENV_FILE" \
    --publish "${PORT}:${PORT}" \
    meep-ipe:latest

echo ""
echo ">>> Done"