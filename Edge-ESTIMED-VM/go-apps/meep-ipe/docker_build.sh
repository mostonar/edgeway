#!/bin/bash

set -e
set -x

SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/meep-ipe

docker pull golang
docker run --rm -it \
    -v"$BASEDIR":/opt/local/etsi/meep-ipe \
    golang bash -c "cd /opt/local/etsi/meep-ipe && ./build-meep-ipe.sh && chown -R $UID:$UID ./bin"

echo ""
echo ">>> Demo Service build completed"

# Stage Dockerfile + data/ folder alongside the binary, matching the
# meepctl dockerize convention (docker-data -> bindir/data -> COPY ./data /).
cp "$BASEDIR/Dockerfile" "$DEMOBIN"
mkdir -p "$DEMOBIN/data"
cp "$BASEDIR/entrypoint.sh" "$DEMOBIN/data/"
cp "$BASEDIR/config.yaml.in" "$DEMOBIN/data/"

docker build -t meep-ipe:latest "$DEMOBIN"

echo ""
echo ">>> Docker image build completed"