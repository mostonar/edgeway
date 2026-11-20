#!/bin/bash

set -e
set +x

docker pull golang
#docker run --rm --expose 80 --expose 443 --publish 80:80 --publish 443:443 -it -v$PWD:/opt/local/etsi/demo10  -v$HOME/var:/opt/local/etsi/var golang bash -c "cd /opt/local/etsi/demo10/bin && ./demo10 ../app_instance.yaml"
docker run --rm -it --publish 9876:9876 -v$PWD:/opt/local/etsi/demo10  -v$HOME/var:/opt/local/etsi/var golang bash -c "cd /opt/local/etsi/demo10/bin && ./demo10"

echo ""
echo ">>> Done"
