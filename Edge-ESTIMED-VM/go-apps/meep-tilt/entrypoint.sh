#!/bin/bash
set -e

echo "=== Initializing meep-tilt container ==="

# Check if GITDIR directory is mounted
if [ -z "$GITDIR" ] || [ ! -d "$GITDIR" ]; then
    echo "ERROR: GITDIR=$GITDIR is not mounted or empty!"
    exit 1
fi

cd "$GITDIR"

echo "=== Setting up meepctl config ==="
mkdir -p /root/.meep
if [ -f "/root/.meepctl.yaml" ]; then
    echo "Found mounted .meepctl.yaml"
else
    echo "Copying repository config file..."
    cp .meepctl-repocfg.yaml /root/.meepctl.yaml
fi

# Ensure correct permissions for the workspace/workdir
if [ -n "$WORKDIR" ]; then
    mkdir -p "$WORKDIR"
fi

echo "=== Starting Tilt ==="
# Start tilt watching the Tiltfile in our sandbox repo, listening on 0.0.0.0 for connection
exec tilt up --host 0.0.0.0 --port 10350 --web-mode=prod -f "$GITDIR/go-apps/meep-tilt/Tiltfile"
