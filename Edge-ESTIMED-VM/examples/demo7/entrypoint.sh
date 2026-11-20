#!/bin/sh
#
# Here are some examples of env. variable values:
#   COMM: --iface eth0 --mode 2
#
#   GLOBAL: --verbose --srcaddr 00:01:02:03:04:05 --rate 1 --start-vam --brocker-address test.mosquitto.org
#
#   SEC_MODE: --no-sec
#
#   BEACON: --no-sec-beacon true
#
#   CAM: --cam-station-type passengerCar --cam-station-id 12345 --cam-no-sec
#
#   DENM 
#
#   VERBOSE: --verbose

# E.g. yann@yann-linux:~/frameworks/fsmsggen$ sudo ./build/x86_64-linux-gnu-d/fsmsggen --iface wlo1 --verbose --srcaddr 00:01:02:03:04:05 --no-sec true --cam-station-type passengerCar --cam-station-id 12345 --cam-no-sec --out ./out.pcap

set -e
set +vx

echo "MEEP_HOST_URL: ${MEEP_HOST_URL}"
echo "MEEP_SANDBOX_NAME: ${MEEP_SANDBOX_NAME}"
echo "MEEP_LOCATION_NAME: ${MEEP_LOCATION_NAME}"

echo "Waiting for MEC plateform started and stable..."
sleep 10 # Wait for MEC plateform started and stable

MQTT_ENABLE=${MQTT_ENABLE:-true}
MQTT_HOST=${MOSQUITTO_NODE_IP_ADDRESS:-"meep-mosquitto"}
MQTT_PORT=${MOSQUITTO_NODE_PORT:-443}
UU_IFACE="--brocker-address $MEEP_HOST_URL/$MEEP_SANDBOX_NAME/$MEEP_LOCATION_NAME/$MQTT_HOST --brocker-port 443 --use-ws --wss-root-ca fullchain.pem --wss-cert chain.pem --wss-key privkey.pem"

echo "Environment variables set:"
echo "MQTT_ENABLE: ${MQTT_ENABLE}"
echo "MQTT_HOST: ${MQTT_HOST}"
echo "MQTT_PORT: ${MQTT_PORT}"
echo "COMM: ${IFACE}"
echo "MODE: ${MODE}"
echo "UU_IFACE: ${UU_IFACE}"
echo "GLOBAL: ${GLOBAL}"
echo "SEC_MODE: ${SEC_MODE}"
echo "BEACON: ${BEACON}"
echo "CAM: ${CAM}"
echo "DENM: ${DENM}"
echo "VERBOSE: ${VERBOSE}"

sleep 10 # Wait for ETSI MEC Platform up and stable

echo "Starting '/fsmsggen ${IFACE} ${MODE} ${UU_IFACE} ${GLOBAL} ${SEC_MODE} ${BEACON} ${CAM} ${DENM} ${VERBOSE}'"
/fsmsggen ${IFACE} ${MODE} ${UU_IFACE} ${GLOBAL} ${SEC_MODE} ${BEACON} ${CAM} ${DENM} ${VERBOSE}