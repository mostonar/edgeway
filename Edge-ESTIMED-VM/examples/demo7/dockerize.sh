#!/bin/bash

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Dockerizing fsmsggen"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

# Dockerize demo 
docker build --no-cache --rm -t meep-docker-registry:30001/fsmsggen .
docker push meep-docker-registry:30001/fsmsggen


echo ""
echo ">>> fsmsggen dockerize completed"




