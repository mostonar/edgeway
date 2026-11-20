#!/bin/bash
set -e

MQTT_ENABLE=${MQTT_ENABLE:-1}

# MEEP / Virt-Enginer injected variables
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

sleep 5 # Wait for ETSI MEC Platform up and stable

# MQTT Configuration
MQTT_HOST=${MQTT_HOST:-${MEP1_MEEP_MOSQUITTO_SERVICE_HOST:-${MEEP_MOSQUITTO_SERVICE_HOST:-"meep-mosquitto"}}}
MQTT_PORT=${MQTT_PORT:-1883}

SERVER_IP="${SERVER_IP:-0.0.0.0}"
SERVER_PORT=${SERVER_PORT:-3002}

CSE_BASE_NAME=${CSE_BASE_NAME:-"TinyIoT"}
CSE_BASE_RI=${CSE_BASE_RI:-"tinyiot"}

echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"

# ------------------------------------------------------------------
# MEC Configuration
# ------------------------------------------------------------------

PLATFORM_ENABLE=${PLATFORM_ENABLE:-1}
PLATFORM_HOST_URL=${SERVER_IP}
PLATFORM_NAME=${MEEP_LOCATION_NAME:-${PLATFORM_NAME:-"mep1"}}
PLATFORM_SANDBOX_ID=${MEEP_SANDBOX_NAME:-"meep-sandbox"}
PLATFORM_APP_INSTANCE_ID=${MEEP_INSTANCE_ID:-"tinyiot-in-cse-instance"}
PLATFORM_HTTP_HOST=${PLATFORM_HTTP_HOST:-"meep-tinyiot-in-cse"}

echo "ENVIRONMENT VARIABLES SET:"
echo "SERVER_IP: ${SERVER_IP}"
echo "SERVER_PORT: ${SERVER_PORT}"
echo "CSE_BASE_NAME: ${CSE_BASE_NAME}"
echo "CSE_BASE_RI: ${CSE_BASE_RI}"

echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "MQTT_USERNAME: ${MQTT_USERNAME}"
echo "MQTT_PASSWORD: ${MQTT_PASSWORD}"

echo "PLATFORM_ENABLE: ${PLATFORM_ENABLE}"
echo "PLATFORM_HOST_URL: ${PLATFORM_HOST_URL}"
echo "PLATFORM_NAME: ${PLATFORM_NAME}"
echo "PLATFORM_SANDBOX_ID: ${PLATFORM_SANDBOX_ID}"
echo "PLATFORM_APP_INSTANCE_ID: ${PLATFORM_APP_INSTANCE_ID}"
echo "PLATFORM_HTTP_HOST: ${PLATFORM_HTTP_HOST}"

# ------------------------------------------------------------------
# Export Variables
# ------------------------------------------------------------------

export MQTT_ENABLE MQTT_HOST MQTT_PORT MQTT_USERNAME MQTT_PASSWORD
export SERVER_PORT SERVER_IP
export CSE_BASE_NAME CSE_BASE_RI
export PLATFORM_ENABLE PLATFORM_HOST_URL PLATFORM_NAME PLATFORM_SANDBOX_ID PLATFORM_APP_INSTANCE_ID PLATFORM_HTTP_HOST

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

if [ "${PLATFORM_ENABLE}" = "1" ]; then
    sleep 5
    echo "Registering to MEC platforms..."

    for host in "mep1-meep-iot" "mep2-meep-iot" "mep3-meep-iot" "mep4-meep-iot"; do
        if getent hosts "${host}" >/dev/null 2>&1; then
            echo "Registering with ${host}..."
            curl -k -s -X POST \
                "http://${host}/iots/v1/registered_iot_platforms" \
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
                                "'"${MEEP_HOST_URL}/${MEEP_SANDBOX_NAME}/${MEEP_LOCATION_NAME}/${PLATFORM_HTTP_HOST}"'"
                            ],
                            "addresses": [{
                                "host": "'"${PLATFORM_NAME}-${PLATFORM_HTTP_HOST}"'",
                                "port": 80
                            }]
                        },
                        "security": {}
                    }],
                    "enabled": true
                }' \
                && echo "MEC registration successful for ${host}" \
                || echo "MEC registration failed for ${host} (non-fatal)"
        fi
    done
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