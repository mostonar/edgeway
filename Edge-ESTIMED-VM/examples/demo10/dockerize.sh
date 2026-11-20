#!/bin/bash

set -e
set +x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Dockerizing Demo10 Server"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

# Copy Dockerfile & config to bin folder
cp $BASEDIR/Dockerfile $DEMOBIN
# cp $BASEDIR/src/backend/app_instance.yaml $DEMOBIN
cp $BASEDIR/entrypoint.sh $DEMOBIN

echo ">>> Dockerizing"
cd $DEMOBIN
docker build --no-cache --rm -t meep-docker-registry:30001/demo10 .
docker push meep-docker-registry:30001/demo10
cd $BASEDIR

echo ""
echo ">>> Done"
