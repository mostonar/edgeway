#!/bin/bash

set -e
set -x

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/meep-ipe/

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Building meep-ipe Go MEC APP"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

# git config --global --add safe.directory "$BASEDIR"

go mod tidy
go build -buildvcs=false -o "$DEMOBIN" .

echo ""
echo ">>> Demo Service build completed"