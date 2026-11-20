#!/bin/bash

# Get full path to script directory
SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Installing redis DB for Unit Testing"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

helm install meep-ut-redis --set meepOrigin="ut" --set master.service.type=NodePort --set master.service.nodePort=30380 $BASEDIR/../charts/data-stores/redis/

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Installing couch DB for Unit Testing"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

helm install meep-ut-couchdb --set meepOrigin="ut" --set service.type=NodePort --set service.nodePort=30985 --set persistentVolume.enabled=false --set persistentVolumeClaim.enabled=false $BASEDIR/../charts/data-stores/couchdb/

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Installing influx DB for Unit Testing"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

helm install meep-ut-influxdb --set meepOrigin="ut" --set service.type=NodePort --set service.apiNodePort=30986 --set service.rpcNodePort=30988 --set persistence.enabled=false --set ingress.enabled=false $BASEDIR/../charts/observability/influxdb/

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Installing Postgis DB for Unit Testing"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

helm install meep-ut-postgis --set meepOrigin="ut" --set securityContext.enabled=false --set master.service.type=NodePort --set master.service.nodePort=30432 --set persistence.enabled=false --set ingress.enabled=false $BASEDIR/../charts/data-stores/postgis/

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Copying Grid Map File"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

sudo cp -rf grid_map.yaml /grid_map.yaml

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Starting meep-gis-asset stub"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

cd $BASEDIR
#go run ./system_test_http_stub.go > /dev/null 2>&1 &
go run ./system_test_http_stub.go > /tmp/system_test_http_stub.log 2>&1 &
RESULT=`ps aux | grep "system_test_http_stub" | grep -v grep | awk {'print$2'}`
echo $RESULT

cd -
