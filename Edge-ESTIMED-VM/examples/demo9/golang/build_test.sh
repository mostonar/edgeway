#!/bin/bash

set -e
set -x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/demo9

docker pull golang
docker run --rm -it -v$PWD:/opt/local/etsi/demo9 golang bash -c "cd /opt/local/etsi/demo9 && ./build-demo9.sh"

echo ""
echo ">>> Demo Service build completed"
