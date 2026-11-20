#!/bin/sh
set -e
SERVICE_NAME="meep-mosquitto"
# Get the namespace from the service account
NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
# Get the token from the service account
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
echo "Token: $TOKEN"
# Query the Kubernetes API to get the NodePort for the service
NODE_PORT=$(curl -sSk \
  -H "Authorization: Bearer $TOKEN" \
  https://kubernetes.default.svc/api/v1/namespaces/$NAMESPACE/services/$SERVICE_NAME \
  | jq -r '.spec.ports[0].nodePort')
echo "External NodePort exposed for service [$SERVICE_NAME] in namespace [$NAMESPACE] is: $NODE_PORT"
exec /usr/sbin/mosquitto -c /mosquitto/config/mosquitto.conf
