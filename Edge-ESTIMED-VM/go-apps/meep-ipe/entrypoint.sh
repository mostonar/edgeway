#!/bin/bash
set -e

# Wait for the MEC platform to stabilize and acme in cse to be ready. A random
# 0-9s offset is added so MEC IPE instances started together in the cluster do
# not all send their first requests at the same moment.
STARTUP_DELAY=$((2 + RANDOM % 10))
echo "Waiting ${STARTUP_DELAY}s before starting meep-ipe"
sleep ${STARTUP_DELAY}

# BASE_PATH — normally derived from sandbox/location (used in the k8s/Helm
# deployment). For standalone/local docker run testing, set
# MEEP_STANDALONE=true to force this empty instead.
if [ "${MEEP_STANDALONE}" == "true" ]; then
    BASE_PATH=""
else
    if [[ ! -z "${MEEP_LOCATION_NAME}" ]]; then
        svcPath="${MEEP_SANDBOX_NAME}/${MEEP_LOCATION_NAME}"
    else
        svcPath="${MEEP_SANDBOX_NAME}"
    fi
    BASE_PATH=${svcPath}
fi

if [ -z "${MEEP_HOST_URL}" ]; then
    echo "$0: MEEP_HOST_URL parameter is not set"
    MEEP_HOST_URL=mec-platform.etsi.org
fi

SERVER_IP="${MEEP_HOST_URL#https://}"
SERVER_IP="${SERVER_IP#http://}"

# IPE's own address — defaults to the sandbox host (SERVER_IP) unless
# explicitly overridden. IPE may run on a different host than the MEC
# Sandbox platform itself.
if [ -z "${MEEP_IPE_URL}" ]; then
    IPE_URL="${SERVER_IP}"
else
    IPE_URL="${MEEP_IPE_URL#https://}"
    IPE_URL="${IPE_URL#http://}"
fi

# IPE port — defaults to 80 unless explicitly overridden.
if [ -z "${MEEP_IPE_PORT}" ]; then
    SERVER_PORT="80"
else
    SERVER_PORT="${MEEP_IPE_PORT}"
fi

SANDBOXNAME=${MEEP_SANDBOX_NAME}
SANDBOXURL="https://${SERVER_IP}"
APPID=${MEEP_INSTANCE_ID}
MECPLATFORM=${MEEP_LOCATION_NAME}
if [ "${MEEP_STANDALONE}" == "true" ]; then
    MEC_PLTF_INSTANCIATED=false
else
    MEC_PLTF_INSTANCIATED=true
fi

# HTTPS flag — defaults to false unless explicitly set to true.
if [ -z "${MEEP_HTTPS}" ]; then
    HTTPS="false"
elif [ "${MEEP_HTTPS}" == "true" ]; then
    HTTPS="true"
else
    HTTPS="false"
fi

# CSE CONFIGS — injected directly at runtime; fall back to sandbox
# defaults if not explicitly provided.
if [ -z "${MEEP_CSE_URL}" ]; then
    CSE_URL="https://${SERVER_IP}"
else
    CSE_URL="${MEEP_CSE_URL}"
fi

if [ -z "${MEEP_CSE_PORT}" ]; then
    CSE_PORT="443/${SANDBOXNAME}/monaco-telecom/meep-acme-in-cse"
else
    CSE_PORT="${MEEP_CSE_PORT}"
fi

if [ -z "${MEEP_CSE_ID}" ]; then
    CSE_ID="laboai-id-in"
else
    CSE_ID="${MEEP_CSE_ID}"
fi

if [ -z "${MEEP_CSE_NAME}" ]; then
    CSE_NAME="laboai-cse-in"
else
    CSE_NAME="${MEEP_CSE_NAME}"
fi

# Optionally append the MEC platform and sandbox name to the CSE ID/name
# (<base>-<mecplatform>-<sandbox>), matching the naming used by meep-acme-mn-cse;
# empty parts are skipped. Enabled only when MEEP_INITIATED_MN_CSE=true.
if [ "${MEEP_INITIATED_MN_CSE}" == "true" ]; then
    for suffix in "${MECPLATFORM}" "${SANDBOXNAME}"; do
        if [ -n "${suffix}" ]; then
            CSE_ID="${CSE_ID}-${suffix}"
            CSE_NAME="${CSE_NAME}-${suffix}"
        fi
    done
fi

# CSE HTTP Basic auth credentials — leave empty if the CSE has no auth
CSE_USER="${MEEP_CSE_USER}"
CSE_PASSWORD="${MEEP_CSE_PASSWORD}"

# AE-ID — base defaults to CMecIPE unless explicitly overridden. The MEC
# platform and sandbox name are appended (<base>-<mecplatform>-<sandbox>) so
# multiple IPE instances can share the same CSE without conflicts; empty
# parts are skipped.
if [ -z "${MEEP_AE_ID}" ]; then
    AE_ID="CMecIPE"
else
    AE_ID="${MEEP_AE_ID}"
fi
if [[ ! "${AE_ID}" =~ ^[A-Za-z0-9_-]+$ ]]; then
    echo "$0: MEEP_AE_ID '${AE_ID}' is invalid, only [A-Za-z0-9_-] is allowed"
    exit 1
fi
# A standalone (outside sandbox) IPE must set its own base AE-ID; falling back to
# the default would share node/subscription names with the in-sandbox IPE on the same CSE.
if [ "${MEEP_STANDALONE}" == "true" ] && [ -z "${MEEP_AE_ID}" ]; then
    echo "$0: MEEP_STANDALONE=true requires MEEP_AE_ID to be set"
    exit 1
fi
if [ -n "${MECPLATFORM}" ]; then
    AE_ID="${AE_ID}-${MECPLATFORM}"
fi
if [ -n "${SANDBOXNAME}" ]; then
    AE_ID="${AE_ID}-${SANDBOXNAME}"
fi

echo "Environment variables set:"
echo "SERVER_IP: ${SERVER_IP}"
echo "IPE_URL: ${IPE_URL}"
echo "SERVER_PORT: ${SERVER_PORT}"
echo "SANDBOXURL: ${SANDBOXURL}"
echo "SANDBOXNAME: ${SANDBOXNAME}"
echo "APPID: ${APPID}"
echo "MECPLATFORM: ${MECPLATFORM}"
echo "MEC_PLTF_INSTANCIATED: ${MEC_PLTF_INSTANCIATED}"
echo "BASE_PATH: ${BASE_PATH}"
echo "HTTPS: ${HTTPS}"

echo "CSE-URL: ${CSE_URL}"
echo "CSE-PORT: ${CSE_PORT}"
echo "CSE-ID: ${CSE_ID}"
echo "CSE-NAME: ${CSE_NAME}"
echo "CSE-USER: ${CSE_USER}"
echo "AE-ID: ${AE_ID}"

export SERVER_IP
export IPE_URL
export SERVER_PORT
export SANDBOXURL
export SANDBOXNAME
export APPID
export MECPLATFORM
export MEC_PLTF_INSTANCIATED
export BASE_PATH
export HTTPS

export CSE_URL
export CSE_PORT
export CSE_ID
export CSE_NAME
export CSE_USER
export CSE_PASSWORD
export AE_ID

workdir="/"
cd "$workdir" || { echo "Directory $workdir not found"; exit 1; }
envsubst < config.yaml.in > config.yaml
cat config.yaml
exec /meep-ipe