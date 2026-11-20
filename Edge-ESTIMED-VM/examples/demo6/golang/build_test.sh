#!/bin/bash

set -e
set -x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/demo6

docker pull golang
docker run --rm -it -v$PWD:/opt/local/etsi/demo6 golang bash -c "cd /opt/local/etsi/demo6 && ./build-demo6.sh"

echo ""
echo ">>> Demo Service build completed"
