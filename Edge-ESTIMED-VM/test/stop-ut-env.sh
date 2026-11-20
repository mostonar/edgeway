#!/bin/bash

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Stopping UT environment"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

helm ls --short | grep meep-ut | xargs -L1 helm uninstall

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Removing Grid Map File"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

sudo rm -rf /grid_map.yaml

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Stopping meep-gis-asset stub"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

PWDX=`ps aux | grep "exe/system_test_http_stub" | grep -v grep | awk {'print$2'}`
if [ ! -z "$PWDX" ]
then
    kill $PWDX
fi
PWDX=`ps aux | grep "exe/dummy_callback_server" | grep -v grep | awk {'print$2'}`
if [ ! -z "$PWDX" ]
then
    kill $PWDX
fi
