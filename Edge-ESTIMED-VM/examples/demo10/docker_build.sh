#!/bin/bash

set -e
set -x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/demo10

docker pull golang
docker run --rm -it -v$PWD:/opt/local/etsi/demo10 golang bash -c "cd /opt/local/etsi/demo10 && ./build-demo10.sh && chown -R $UID:$UID ./bin"

echo ""
echo ">>> Demo Service build completed"
