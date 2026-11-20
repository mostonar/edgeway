#!/bin/bash
set -e

usage() {
    cat <<EOF
Generate self signed certificate & secret.

usage: ${0} [OPTIONS]

The following flags are required.

       --service          Service name of registry.
       --namespace        Namespace where registry service and secret reside.
       --secret           Secret name for CA certificate and server certificate/key pair.
       --certdir          Directory where certificates should be stored.
       --host             Host name or IP address for SAN.
EOF
    exit 1
}

while [[ $# -gt 0 ]]; do
    case ${1} in
        --service)
            service="$2"
            shift
            ;;
        --secret)
            secret="$2"
            shift
            ;;
        --namespace)
            namespace="$2"
            shift
            ;;
        --certdir)
            certdir="$2"
            shift
            ;;
        --host)
            host="$2"
            shift
            ;;
        *)
            usage
            ;;
    esac
    shift
done

[ -z ${service} ] && service=meep-ingress
[ -z ${secret} ] && secret=meep-ingress
[ -z ${namespace} ] && namespace=default
[ -z ${certdir} ] && certdir=$(mktemp -d)
[ -z ${host} ] && host="192.168.20.167"

if [ ! -x "$(command -v openssl)" ]; then
    echo "openssl not found"
    exit 1
fi

if [[ "$host" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    alt_name="IP:${host}"
else
    alt_name="DNS:${host}"
fi

echo "creating certs in certdir: ${certdir} for host: ${host} with SAN: ${alt_name}"
openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout ${certdir}/${service}.key -out ${certdir}/${service}.pem -subj "/CN=${host}/O=InterDigital" -addext "subjectAltName = ${alt_name}"

echo "creating secret: ${namespace}/${secret}"
kubectl create secret tls ${secret} \
    --key ${certdir}/${service}.key \
    --cert ${certdir}/${service}.pem \
    --dry-run -o yaml | 
    kubectl -n ${namespace} apply -f -