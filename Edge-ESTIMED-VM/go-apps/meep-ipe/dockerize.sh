#!/bin/bash

set -e
set +x

SCRIPT=$(readlink -f "$0")
BASEDIR=$(dirname "$SCRIPT")

DEMOBIN=$BASEDIR/bin/meep-ipe

echo ""
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ">>> Dockerizing MEEP IPE Server"
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
echo ""

if [ ! -f "$DEMOBIN/meep-ipe" ]; then
    echo ">>> Binary not found at $DEMOBIN/meep-ipe — building it now"
    docker pull golang
    docker run --rm -it \
        -v"$BASEDIR":/opt/local/etsi/meep-ipe \
        golang bash -c "cd /opt/local/etsi/meep-ipe && ./build-meep-ipe.sh && chown -R $UID:$UID ./bin"
else
    echo ">>> Using existing binary at $DEMOBIN/meep-ipe"
fi

# Stage Dockerfile + data/ folder alongside the binary, matching the
# same convention meepctl's dockerize command uses.
cp "$BASEDIR/Dockerfile" "$DEMOBIN"
mkdir -p "$DEMOBIN/data"
cp "$BASEDIR/entrypoint.sh" "$DEMOBIN/data/"
cp "$BASEDIR/config.yaml.in" "$DEMOBIN/data/"

echo ">>> Dockerizing"
cd "$DEMOBIN"
docker build --no-cache --rm -t meep-docker-registry:30001/meep-ipe .
docker push meep-docker-registry:30001/meep-ipe
cd "$BASEDIR"

echo ""
echo ">>> Done"