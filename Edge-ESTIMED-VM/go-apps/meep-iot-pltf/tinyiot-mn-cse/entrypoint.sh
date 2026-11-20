#!/bin/bash
set -e

MQTT_ENABLE=${MQTT_ENABLE:-1}

# ------------------------------------------------------------------
# MEEP / MEC injected variables
# ------------------------------------------------------------------
echo "Using Helm-injected and Kubernetes auto-injected environment variables instead of API queries"
echo "MEEP_HOST_URL: ${MEEP_HOST_URL}"
echo "MEEP_LOCATION_NAME: ${MEEP_LOCATION_NAME}"
echo "MEEP_SANDBOX_NAME: ${MEEP_SANDBOX_NAME}"
echo "MEEP_INSTANCE_ID: ${MEEP_INSTANCE_ID}"

# Normalize host without modifying original MEEP_HOST_URL
SERVER_IP="${MEEP_HOST_URL#http://}"
SERVER_IP="${SERVER_IP#https://}"

if [[ ! -z "${MEEP_LOCATION_NAME}" ]]; then
    svcPath="${MEEP_SANDBOX_NAME}/${MEEP_LOCATION_NAME}"
else
    svcPath="${MEEP_SANDBOX_NAME}"
fi

if [ -z "${MEEP_HOST_URL}" ]; then
    echo "$0: MEEP_HOST_URL parameter is not set"
    MEEP_HOST_URL="mec-platform.etsi.org"
fi

sleep 30 # Wait for ETSI MEC Platform and IN-CSE to be up and stable

# ------------------------------------------------------------------
# MQTT Configuration
# ------------------------------------------------------------------
MQTT_HOST=${MQTT_HOST:-${MEP1_MEEP_MOSQUITTO_SERVICE_HOST:-${MEEP_MOSQUITTO_SERVICE_HOST:-"meep-mosquitto"}}}
MQTT_PORT=${MQTT_PORT:-1883}

SERVER_IP="${SERVER_IP:-0.0.0.0}"
SERVER_PORT=${SERVER_PORT:-3005}

CSE_BASE_NAME=${CSE_BASE_NAME:-"id-mn"}
CSE_BASE_RI=${CSE_BASE_RI:-"id-mn"}

MQTT_USERNAME=${MQTT_USERNAME:-"tinyiot-mn-cse"}
MQTT_PASSWORD=${MQTT_PASSWORD:-"mqtt"}

echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"

# ------------------------------------------------------------------
if [ -z "${REMOTE_CSE_HOST}" ]; then
    # Try different potential hostnames for IN-CSE
    for host in "monaco-telecom-meep-tinyiot-in-cse" "${MEEP_LOCATION_NAME}-meep-tinyiot-in-cse" "mep1-meep-tinyiot-in-cse" "mep2-meep-tinyiot-in-cse" "meep-tinyiot-in-cse"; do
        if [ ! -z "${host}" ]; then
            echo "Checking resolution for: ${host}"
            if getent hosts "${host}" >/dev/null 2>&1; then
                REMOTE_CSE_HOST="${host}"
                echo "Found resolvable remote CSE host: ${REMOTE_CSE_HOST}"
                break
            fi
        fi
    done
    # Fallback to default if none resolved
    REMOTE_CSE_HOST=${REMOTE_CSE_HOST:-"${MEEP_LOCATION_NAME:-"mep1"}-meep-tinyiot-in-cse"}
fi
REMOTE_CSE_PORT=${REMOTE_CSE_PORT:-80}

# Resolve REMOTE_CSE_HOST to IP address since TinyIoT HTTP client doesn't support DNS resolution
if [[ ! "$REMOTE_CSE_HOST" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Resolving REMOTE_CSE_HOST hostname to IP: ${REMOTE_CSE_HOST}"
    RESOLVED_IP=$(getent hosts "${REMOTE_CSE_HOST}" | awk '{print $1}')
    if [ ! -z "${RESOLVED_IP}" ]; then
        REMOTE_CSE_HOST="${RESOLVED_IP}"
        echo "Resolved to IP: ${REMOTE_CSE_HOST}"
    else
        echo "Warning: Could not resolve hostname ${REMOTE_CSE_HOST} to IP"
    fi
fi

# ------------------------------------------------------------------
# MEC Configuration
# ------------------------------------------------------------------
MEC_ENABLE=${MEC_ENABLE:-1}
MEC_HOST_URL=${SERVER_IP}
[ -z "$MEC_HOST_URL" ] && MEC_HOST_URL="mec-platform.etsi.org"

MEC_PLATFORM=${MEEP_LOCATION_NAME:-${MEC_PLATFORM:-"mep1"}}
MEC_SANDBOX_ID=${MEEP_SANDBOX_NAME:-"meep-sandbox"}
MEC_APP_INSTANCE_ID=${MEEP_INSTANCE_ID:-"tinyiot-mn-cse-instance"}
MEC_HTTP_HOST=${MEC_HTTP_HOST:-"meep-tinyiot-mn-cse"}

# ------------------------------------------------------------------
# Environment Dump
# ------------------------------------------------------------------
echo "ENVIRONMENT VARIABLES SET:"
echo "SERVER_IP: ${SERVER_IP}"
echo "SERVER_PORT: ${SERVER_PORT}"
echo "CSE_BASE_NAME: ${CSE_BASE_NAME}"
echo "CSE_BASE_RI: ${CSE_BASE_RI}"
echo "REMOTE_CSE_HOST: ${REMOTE_CSE_HOST}"
echo "REMOTE_CSE_PORT: ${REMOTE_CSE_PORT}"
echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"
echo "MEC_ENABLE: ${MEC_ENABLE}"
echo "MEC_HOST_URL: ${MEC_HOST_URL}"
echo "MEC_PLATFORM: ${MEC_PLATFORM}"
echo "MEC_SANDBOX_ID: ${MEC_SANDBOX_ID}"
echo "MEC_APP_INSTANCE_ID: ${MEC_APP_INSTANCE_ID}"
echo "MEC_HTTP_HOST: ${MEC_HTTP_HOST}"

# ------------------------------------------------------------------
# Export Variables
# ------------------------------------------------------------------

export MQTT_ENABLE MQTT_HOST MQTT_PORT MQTT_USERNAME MQTT_PASSWORD
export SERVER_PORT SERVER_IP
export CSE_BASE_NAME CSE_BASE_RI
export REMOTE_CSE_HOST REMOTE_CSE_PORT
export MEC_ENABLE MEC_HOST_URL MEC_PLATFORM MEC_SANDBOX_ID MEC_APP_INSTANCE_ID MEC_HTTP_HOST

# ------------------------------------------------------------------
# Setup Working Directory
# ------------------------------------------------------------------
workdir="/usr/src/app/tinyIoT/source/server"
cd "$workdir" || {
    echo "Directory $workdir not found"
    exit 1
}

# ------------------------------------------------------------------
# Start TinyIoT Server
# ------------------------------------------------------------------
./server &
SERVER_PID=$!

# ------------------------------------------------------------------
# Register to MEC Platform
# ------------------------------------------------------------------
if [ "${MEC_ENABLE}" = "1" ]; then
    sleep 5
    echo "Registering to MEC platform..."

    curl -s -X POST \
        "http://${MEC_PLATFORM}-meep-iot/iots/v1/registered_iot_platforms" \
        -H "Content-Type: application/json" \
        -d '{
            "iotPlatformId": "'"${CSE_BASE_RI}"'",
            "userTransportInfo": [{
                "id": "'"${CSE_BASE_RI}"'-mqtt-01",
                "name": "'"${CSE_BASE_RI}"'",
                "description": "TinyIoT CSE MQTT",
                "type": "MB_TOPIC_BASED",
                "protocol": "MQTT",
                "version": "3.1.1",
                "endpoint": {
                    "addresses": [{
                        "host": "'"${MQTT_HOST}"'",
                        "port": '"${MQTT_PORT}"'
                    }]
                },
                "security": {
                    "username": "'"${MQTT_USERNAME}"'",
                    "password": "'"${MQTT_PASSWORD}"'",
                    "useTLS": false
                },
                "implSpecificInfo": {
                    "eventTopics": [
                        "/oneM2M/req/+/'"${CSE_BASE_RI}"'/#"
                    ],
                    "uplinkTopics": [
                        "/oneM2M/resp/'"${CSE_BASE_RI}"'/+/#"
                    ],
                    "downlinkTopics": [
                        "/oneM2M/reg_req/+/'"${CSE_BASE_RI}"'/#",
                        "/oneM2M/reg_resp/'"${CSE_BASE_RI}"'/+/#"
                    ]
                }
            }],
            "customServicesTransportInfo": [{
                "id": "'"${CSE_BASE_RI}"'-http-01",
                "name": "'"${CSE_BASE_NAME}"'",
                "description": "TinyIoT CSE HTTP",
                "type": "REST_HTTP",
                "protocol": "REST_HTTP",
                "version": "1.1",
                "endpoint": {
                    "uris": [
                        "'"${MEEP_HOST_URL}/${MEEP_SANDBOX_NAME}/${MEEP_LOCATION_NAME}/${MEC_HTTP_HOST}"'"
                    ],
                    "addresses": [{
                        "host": "'"${MEC_PLATFORM}-${MEC_HTTP_HOST}"'",
                        "port": 80
                    }]
                },
                "security": {}
            }],
            "enabled": true
        }' \
        && echo "MEC registration successful" \
        || echo "MEC registration failed (non-fatal)"
fi

# ------------------------------------------------------------------
# Wait for Server Process
# ------------------------------------------------------------------
trap_term() {
  echo "Caught SIGTERM signal!"
  kill -TERM "$SERVER_PID" 2>/dev/null
  wait "$SERVER_PID"
  exit 0
}
trap trap_term SIGTERM SIGINT

wait $SERVER_PID