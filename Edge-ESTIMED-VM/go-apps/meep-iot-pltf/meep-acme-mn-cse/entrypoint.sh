#!/bin/bash
set -e

echo "MEEP_HOST_URL: ${MEEP_HOST_URL}"
# SERVER_IP="${MEEP_HOST_URL#http://}"; MEEP_HOST_URL="${MEEP_HOST_URL#https://}"
SERVER_IP="${MEEP_HOST_URL#http://}"
SERVER_IP="${SERVER_IP#https://}"

echo "MEEP_HOST_URL: ${MEEP_HOST_URL}"
echo "MEC_PLATFORM=${MEEP_LOCATION_NAME}"
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
echo "APP_INSTANCE_ID: ${MEEP_INSTANCE_ID}"

sleep 10 # Wait for the MEC platform to stablize and acme in cse to be ready

# Retrieve the internal meep-mosquitto IP address
SERVICE_NAME="meep-mosquitto"
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

# # Retrieve the internal meep-acme-in-cse IP address
# SERVICE_NAME="meep-acme-in-cse"
# REMOTE_CSE_IP_ADDRESS=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
#   | jq -r '.spec.clusterIP')
# echo "Internal IP exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $REMOTE_CSE_IP_ADDRESS"

# # Retrieve the internal meep-acme-in-cse port id
# SERVICE_NAME="meep-acme-in-cse"
# NODE_PORT_IN_CSE=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
#   | jq -r '.spec.ports[0].targetPort')
# echo "Internal NodePort exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $NODE_PORT_IN_CSE"

# SERVICE_NAME="meep-acme-mn-cse"
# NODE_IP=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
#   | jq -r '.spec.clusterIP')
# echo "External NodePort exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $NODE_IP"
# NODE_PORT=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
#   | jq -r '.spec.ports[0].nodePort')
# echo "External NodePort exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $NODE_PORT"

# The following name is the one name used to represent its block in scenario at endpoint /alt
# ACME_IN_SERVICE_NAME=${ACME_IN_SERVICE_NAME:-"monaco-telecom-meep-acme-in-cse"}
# # Fetch pod name acme in cse
# POD_NAME=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/pods \
#   | jq -r --arg svc "$ACME_IN_SERVICE_NAME" '.items[] | select(.metadata.labels.app == $svc) | .metadata.name' | head -n 1)
# echo "Pod name for service [$ACME_IN_SERVICE_NAME] in namespace [$NAMESPACE] is: $POD_NAME"

# # Step 2: Get the MEEP_LOCATION_NAME of ACME IN CSE environment variable from the pod
# ACME_MEEP_LOCATION_NAME=$(curl -sSk \
#   -H "Authorization: Bearer $TOKEN" \
#   https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/pods/$POD_NAME \
#   | jq -r '.spec.containers[0].env[] | select(.name=="MEEP_LOCATION_NAME") | .value')
# echo "MEEP_LOCATION_NAME for service [$ACME_IN_SERVICE_NAME] in namespace [$NAMESPACE] is: $ACME_MEEP_LOCATION_NAME"

if [[ ! -z "${MEEP_LOCATION_NAME}" ]]; then
    svcPath="${MEEP_SANDBOX_NAME}/${MEEP_LOCATION_NAME}"
    echo "SVC_PATH inside loop : ${MEEP_LOCATION_NAME}"
else
    svcPath="${MEEP_SANDBOX_NAME}"
fi

if [ "${MEEP_HOST_URL}" == "" ]
then
    echo "$0: MEEP_HOST_URL parameter is not set"
    MEEP_HOST_URL=mec-platform.etsi.org
fi

SERVER_IP=`echo $SERVER_IP | sed -e "s/https:\/\///g"` # Remove https://
SERVER_TYPE=${SERVER_TYPE:-"MN"}
SERVER_PORT=${SERVER_PORT:-3004}

# MN-CSE Address

CSE_BASE_NAME=${CSE_BASE_NAME:-"mep-cse-mn"}
CSE_BASE_RI=${CSE_BASE_RI:-"acme-mep-id-mn-cse"}
# Append location and sandbox names (skipping empty ones) to make this CSE's ID/name unique
for suffix in "${MEEP_LOCATION_NAME}" "${MEEP_SANDBOX_NAME}"; do
    if [[ -n "${suffix}" ]]; then
        CSE_BASE_NAME="${CSE_BASE_NAME}-${suffix}"
        CSE_BASE_RI="${CSE_BASE_RI}-${suffix}"
    fi
done
MN_CSE_FULL_ADDRESS="https://${SERVER_IP}/${svcPath}/meep-acme-mn-cse"
# REMOTE_CSE_HOST="${REMOTE_CSE_IP_ADDRESS:-"meep-acme-in-cse"}"
# REMOTE_CSE_PORT=${NODE_PORT_IN_CSE:-3003}
if [[ -n "${EXTERNAL_REMOTE_CSE_HOST}" ]]; then
    # Register with a publicly accessible (non-cluster) IN-CSE
    echo "Using external remote CSE: ${EXTERNAL_REMOTE_CSE_HOST}"
    REMOTE_CSE_HOST=${EXTERNAL_REMOTE_CSE_HOST}
    REMOTE_CSE_PORT=${EXTERNAL_REMOTE_CSE_PORT:-443}
    REMOTE_CSE_PROTOCOL=${EXTERNAL_REMOTE_CSE_PROTOCOL:-"https"}
    # Strip trailing slashes: ACME builds the registrar URL as address + root + "/", so a
    # trailing slash yields "///<cse-id>" which is parsed as an absolute ID (/_/) and rejected
    REMOTE_SVC_PATH=$(echo "${EXTERNAL_REMOTE_SVC_PATH}" | sed 's:/*$::')
else
    REMOTE_CSE_HOST=${SERVER_IP:-"meep-acme-in-cse"}
    # REMOTE_CSE_PORT=443/${MEEP_SANDBOX_NAME}/${ACME_MEEP_LOCATION_NAME:-"monaco-telecom"}/meep-acme-in-cse
    REMOTE_CSE_PORT=443
    REMOTE_CSE_PROTOCOL="https"
    REMOTE_SVC_PATH=/${MEEP_SANDBOX_NAME}/${ACME_MEEP_LOCATION_NAME:-"monaco-telecom"}/meep-acme-in-cse
fi
REMOTE_CSE_ID=${REMOTE_CSE_ID:-"laboai-id-in"}
REMOTE_CSE_NAME=${REMOTE_CSE_NAME:-"laboai-cse-in"}
# Credentials for a protected remote CSE (empty = no authentication)
REMOTE_CSE_USER=${REMOTE_CSE_USER:-""}
REMOTE_CSE_PASS=${REMOTE_CSE_PASS:-""}
ENABLE_COAP=${ENABLE_COAP:-false}
ENABLE_WS=${ENABLE_WS:-false}

CSE_EXTERNAL_IP=${SERVER_IP}
CSE_EXTERNAL_PORT=${NODE_PORT:-443}

# MEC Configuration
MEC_ENABLE=${MEC_ENABLE:-true}
MEC_HOST_URL=${SERVER_IP}
MEC_PLATFORM=${MEEP_LOCATION_NAME:-"mep1"}
MEC_SANDBOX_ID=${MEEP_SANDBOX_NAME:-"meep-sandbox"}
MEC_APP_INSTANCE_ID=${MEEP_INSTANCE_ID:-"meep-acme-mn-cse-instance"}

# MQTT Configuration
MQTT_ENABLE=${MQTT_ENABLE:-true}
MQTT_HOST=${SERVER_IP:-"meep-mosquito"}
MQTT_PORT=443
MQTT_USERNAME=${MQTT_USERNAME:-"acme-mn-cse"}
MQTT_PASSWORD=${MQTT_PASSWORD:-"mqtt"}

WEBSOCK_ETPATH="/$MEC_SANDBOX_ID"${WEBSOCK_ETPATH:-"/monaco-telecom/meep-mosquitto"}

SVC_PATH="${svcPath}/meep-acme-mn-cse"

echo "Environment variables set:"
echo "SERVER_TYPE: ${SERVER_TYPE}"
echo "SERVER_IP: ${SERVER_IP}"
echo "SERVER_PORT: ${SERVER_PORT}"
echo "CSE_BASE_NAME: ${CSE_BASE_NAME}"
echo "CSE_BASE_RI: ${CSE_BASE_RI}"
echo "REMOTE_CSE_ID: ${REMOTE_CSE_ID}"
echo "REMOTE_CSE_NAME: ${REMOTE_CSE_NAME}"
echo "REMOTE_CSE_HOST: ${REMOTE_CSE_HOST}"
echo "REMOTE_CSE_PORT: ${REMOTE_CSE_PORT}"
echo "REMOTE_CSE_PROTOCOL: ${REMOTE_CSE_PROTOCOL}"
echo "REMOTE_CSE_USER: ${REMOTE_CSE_USER}"
echo "REMOTE_CSE_PASS: ${REMOTE_CSE_PASS:+****}"
echo "CSE_EXTERNAL_IP: ${CSE_EXTERNAL_IP}"
echo "CSE_EXTERNAL_PORT: ${CSE_EXTERNAL_PORT}"
echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"
echo "MEC_ENABLE: ${MEC_ENABLE}"
echo "MEC_HOST_URL: ${MEC_HOST_URL}"
echo "MEC_PLATFORM: ${MEEP_LOCATION_NAME}"
echo "MEC_SANDBOX_ID: ${MEC_SANDBOX_ID}"
echo "ENABLE_COAP: ${ENABLE_COAP}"
echo "ENABLE_WS: ${ENABLE_WS}"
echo "WEBSOCK_ETPATH: ${WEBSOCK_ETPATH}"
echo "MN_CSE_FULL_ADDRESS: ${MN_CSE_FULL_ADDRESS}"
echo "MEC_APP_INSTANCE_ID: ${MEC_APP_INSTANCE_ID}"
echo "SVC_PATH: ${SVC_PATH}"
echo "REMOTE_SVC_PATH: ${REMOTE_SVC_PATH}"

export SERVER_TYPE
export SERVER_IP
export SERVER_PORT
export MN_CSE_FULL_ADDRESS
export MEC_APP_INSTANCE_ID

export CSE_BASE_NAME
export CSE_BASE_RI
export REMOTE_CSE_ID
export REMOTE_CSE_NAME
export REMOTE_CSE_HOST
export REMOTE_CSE_PORT
export REMOTE_CSE_PROTOCOL
export REMOTE_CSE_USER
export REMOTE_CSE_PASS
export REMOTE_SVC_PATH
export CSE_EXTERNAL_IP
export CSE_EXTERNAL_PORT
export ENABLE_COAP
export ENABLE_WS

export MQTT_ENABLE
export MQTT_HOST
export MQTT_PORT
export MQTT_USERNAME
export MQTT_PASSWORD
export WEBSOCK_ETPATH

export MEC_ENABLE
export MEC_HOST_URL
export MEC_PLATFORM
export MEC_SANDBOX_ID

export SVC_PATH

workdir="/usr/src/app/ACME-oneM2M-CSE"
cd "$workdir" || { echo "Directory $workdir not found"; exit 1; }
envsubst < acme.ini.in > acme.ini
cat acme.ini
exec python3 -m acmecse
