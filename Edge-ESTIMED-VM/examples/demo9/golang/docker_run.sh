#!/bin/bash

set -e
set +x

docker pull golang
docker run --rm -it -v$PWD:/opt/local/etsi/demo9  -v$HOME/var:/opt/local/etsi/var golang bash -c "cd /opt/local/etsi/demo9/bin && ./demo9 ../app_instance.yaml"

echo ""
echo ">>> Done"
