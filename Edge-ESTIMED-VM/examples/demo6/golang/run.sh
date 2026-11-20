#!/bin/bash

set -e
set +x

docker run -it --rm --expose 80/tcp meep-docker-registry:30001/demo6

echo ""
echo ">>> Done"
