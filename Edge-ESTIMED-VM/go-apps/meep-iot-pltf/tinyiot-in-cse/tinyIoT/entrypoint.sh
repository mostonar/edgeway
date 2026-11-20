#!/bin/bash
set -e

MQTT_ENABLE=${MQTT_ENABLE:-1}
SERVER_IP="${SERVER_IP:-192.168.0.112}"

echo "Using Helm-injected and Kubernetes auto-injected environment variables instead of API queries"

if [[ ! -z "${MEEP_LOCATION_NAME}" ]]; then
    svcPath="${MEEP_SANDBOX_NAME}/${MEEP_LOCATION_NAME}"
else
    svcPath="${MEEP_SANDBOX_NAME}"
fi

if [ "${MEEP_HOST_URL}" == "" ]
then
    echo "$0: MEEP_HOST_URL parameter is not set"
    MEEP_HOST_URL=mec-platform.etsi.org
fi

sleep 5 # Wait for ETSI MEC Platform up and stable
MQTT_ENABLE=${MQTT_ENABLE:-1}
MQTT_HOST=${MQTT_HOST:-${MEEP_MOSQUITTO_SERVICE_HOST:-"meep-mosquitto"}}
MQTT_PORT=${MQTT_PORT:-${MEEP_MOSQUITTO_SERVICE_PORT:-1883}}
SERVER_IP=${SERVER_IP:-"0.0.0.0"}
[ -z "$SERVER_IP" ] && SERVER_IP="0.0.0.0"
SERVER_PORT=${SERVER_PORT:-3002}
[ -z "$SERVER_PORT" ] && SERVER_PORT="3002"
ENABLE_COAP=${ENABLE_COAP:-0}
CSE_BASE_NAME=${CSE_BASE_NAME:-"TinyIoT"}
CSE_BASE_RI=${CSE_BASE_RI:-"tinyiot"}
SERVER_TYPE=${SERVER_TYPE:-IN_CSE}
[ -z "$SERVER_TYPE" ] && SERVER_TYPE="IN_CSE"
MQTT_USERNAME=${MQTT_USERNAME:-"tinyiot-in-cse"}
MQTT_PASSWORD=${MQTT_PASSWORD:-"mqtt"}

# MEC Configuration
MEC_ENABLE=${MEC_ENABLE:-1}
MEC_HOST_URL=${MEEP_HOST_URL:-"meep-platform.etsi.org"}
[ -z "$MEC_HOST_URL" ] && MEC_HOST_URL="meep-platform.etsi.org"
MEC_PLATFORM=${MEC_PLATFORM:-"mep1"}
MEC_SANDBOX_ID=${MEEP_SANDBOX_NAME:-"meep-sandbox"}

echo "ENVIRONMENT VARIABLES SET:"
echo "SERVER_IP: ${SERVER_IP}"
echo "SERVER_TYPE: ${SERVER_TYPE}"
echo "SERVER_PORT: ${SERVER_PORT}"
echo "CSE_BASE_NAME: ${CSE_BASE_NAME}"
echo "CSE_BASE_RI: ${CSE_BASE_RI}"
echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"
echo "MEC_ENABLE: ${MEC_ENABLE}"
echo "MEC_HOST_URL: ${MEC_HOST_URL}"
echo "MEC_PLATFORM: ${MEC_PLATFORM}"
echo "MEC_SANDBOX_ID: ${MEC_SANDBOX_ID}"

export MQTT_ENABLE
export MQTT_HOST
export MQTT_PORT
export MQTT_USERNAME
export MQTT_PASSWORD
export SERVER_PORT
export SERVER_TYPE
export SERVER_IP
export ENABLE_COAP
export CSE_BASE_NAME
export CSE_BASE_RI
export MEC_ENABLE
export MEC_HOST_URL
export MEC_PLATFORM
export MEC_SANDBOX_ID

workdir="/usr/src/app/tinyIoT/source/server"
cd "$workdir" || { echo "Directory $workdir not found"; exit 1; }
envsubst < config.h.in > config.h
make clean && make
./server
