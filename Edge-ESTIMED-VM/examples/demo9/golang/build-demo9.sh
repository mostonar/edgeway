#!/bin/bash

set -e
set -x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/demo9

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Building Demo 9 Go MEC APP"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

go mod tidy
go build -o $DEMOBIN .

echo ""
echo ">>> Demo Service build completed"
