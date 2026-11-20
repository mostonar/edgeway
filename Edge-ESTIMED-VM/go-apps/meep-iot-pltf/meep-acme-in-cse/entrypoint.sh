#!/bin/bash
set -e

echo "MEEP_HOST_URL: ${MEEP_HOST_URL}"
# SERVER_IP="${MEEP_HOST_URL#http://}"; MEEP_HOST_URL="${MEEP_HOST_URL#https://}"
SERVER_IP="${MEEP_HOST_URL#http://}"
SERVER_IP="${SERVER_IP#https://}"

echo "MEC_PLATFORM=$MEC_PLATFORM"
echo "MEEP_SANDBOX_NAME=$MEEP_SANDBOX_NAME"

# Other environment variables
echo "SERVER_TYPE: ${SERVER_TYPE}"
echo "SERVER_PORT: ${SERVER_PORT}"

echo "CSE_BASE_NAME: ${CSE_BASE_NAME}"
echo "CSE_BASE_RI: ${CSE_BASE_RI}"

echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"

# Retrieve the internal meep-mosquitto IP address
SERVICE_NAME="monaco-telecom-meep-cloud-mosquitto"
NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
# MOSQUITTO_NODE_IP_ADDRESS=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
#   | jq -r '.spec.clusterIP')
# echo "Internal IP exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $MOSQUITTO_NODE_IP_ADDRESS"

# # Retrieve the internal meep-mosquitto port id
# MOSQUITTO_NODE_PORT=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
#   | jq -r '.spec.ports[0].targetPort')
# echo "Internal NodePort exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $MOSQUITTO_NODE_PORT"

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

SVC_PATH="${svcPath}/meep-acme-in-cse"

SERVER_IP=`echo $SERVER_IP | sed -e "s/https:\/\///g"` # Remove https://
SERVER_PORT=${SERVER_PORT:-3003}
MEC_SANDBOX_ID=${MEEP_SANDBOX_NAME:-"meep-sandbox"}

CSE_BASE_NAME=${CSE_BASE_NAME:-"laboai-cse-in"}
CSE_BASE_RI=${CSE_BASE_RI:-"laboai-id-in"}
ENABLE_COAP=${ENABLE_COAP:-false}
ENABLE_WS=${ENABLE_WS:-false}

# MQTT Configuration
MQTT_ENABLE=${MQTT_ENABLE:-true}
MQTT_HOST=${SERVER_IP:-"monaco-telecom-meep-cloud-mosquitto"}
MQTT_PORT=${MOSQUITTO_NODE_PORT:-443}
MQTT_USERNAME=${MQTT_USERNAME:-"acme-mn-cse"}
MQTT_PASSWORD=${MQTT_PASSWORD:-"mqtt"}

WEBSOCK_ETPATH="/$MEC_SANDBOX_ID"${WEBSOCK_ETPATH:-"/monaco-telecom/meep-mosquitto"}

echo "Environment variables set:"
echo "SERVER_TYPE: ${SERVER_TYPE}"
echo "SERVER_IP: ${SERVER_IP}"
echo "SERVER_PORT: ${SERVER_PORT}"
echo "CSE_BASE_NAME: ${CSE_BASE_NAME}"
echo "CSE_BASE_RI: ${CSE_BASE_RI}"
echo "ENABLE_COAP: ${ENABLE_COAP}"
echo "ENABLE_WS: ${ENABLE_WS}"
echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"
echo "WEBSOCK_ETPATH: ${WEBSOCK_ETPATH}"
echo "SVC_PATH: ${SVC_PATH}"

export SERVER_TYPE
export SERVER_IP
export SERVER_PORT

export CSE_BASE_NAME
export CSE_BASE_RI
export ENABLE_COAP
export ENABLE_WS

export MQTT_ENABLE
export MQTT_HOST
export MQTT_PORT
export MQTT_USERNAME
export MQTT_PASSWORD
export WEBSOCK_ETPATH

export SVC_PATH

workdir="/usr/src/app/ACME-oneM2M-CSE"
cd "$workdir" || { echo "Directory $workdir not found"; exit 1; }
envsubst < acme.ini.in > acme.ini
cat acme.ini
exec python3 -m acmecse
