#!/bin/bash

set -e
set +x

docker pull golang
#docker run --rm --expose 80 --expose 443 --publish 80:80 --publish 443:443 -it -v$PWD:/opt/local/etsi/demo6  -v$HOME/var:/opt/local/etsi/var golang bash -c "cd /opt/local/etsi/demo6/bin && ./demo6 ../app_instance.yaml"
docker run --rm -it -v$PWD:/opt/local/etsi/demo6  -v$HOME/var:/opt/local/etsi/var golang bash -c "cd /opt/local/etsi/demo6/bin && ./demo6 ../app_instance.yaml"

echo ""
echo ">>> Done"
