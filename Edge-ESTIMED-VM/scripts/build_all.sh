#!/bin/sh -e

#set -vx

DESC="etsi-mec-sandbox platform"
TIMEOUT=7
IS_RUNNING=`kubectl get pods --all-namespaces | grep "meep-virt-engine" | awk '{print $2}'`
ADV_PATH=~/etsi-mec-sandbox
DO_NOT_BUILD_EX=$1

stop_etsi_mec_sandbox() {
    if [ -n $IS_RUNNING ]; then
    	meepctl delete core
	sleep $TIMEOUT
	meepctl delete dep
	sleep $TIMEOUT
	kubectl get pods --all-namespaces
	sleep $TIMEOUT
    #helm delete --no-hooks meep-prometheus
    fi
}

build_etsi_mec_sandbox() {
    meepctl deploy dep all
    sleep $TIMEOUT
    rm -fr $ADV_PATH/bin/meep-*
    if [ "$DO_NOT_BUILD_EX" != "--noexamplesbuilt" ]; then
        build_examples
    fi
    meepctl build all --nolint
    meepctl dockerize all
    meepctl delete dep
    sleep $TIMEOUT
}

build_examples() {
    cd $ADV_PATH/examples/demo1
    ./build-demo1.sh && ./dockerize.sh
    cd $ADV_PATH/examples/demo2
    ./build-demo2.sh &&
    cd $ADV_PATH/examples/demo3
    ./build-demo3.sh && ./dockerize.sh
    cd $ADV_PATH/examples/demo4-ue
    ./build-demo4-ue.sh && ./dockerize.sh
    cd $ADV_PATH/examples/demo6/golang
    ./docker_build.sh && ./dockerize.sh
    cd $ADV_PATH/examples/demo6/python
    #./docker_build.sh && ./dockerize.sh
    cd $ADV_PATH/examples/demo9/golang
    ./docker_build.sh && ./dockerize.sh
    cd $ADV_PATH/examples/demo10
    ./docker_build.sh && ./dockerize.sh
}

usage() {
    echo "Usage: $0 [--noexamplesbuilt]" >&2
    echo "       --noexamplesbuilt: To exclude examples from build process (optional)"
    exit 1
}

if [ "$DO_NOT_BUILD_EX" != "" ]; then
    if [ "$DO_NOT_BUILD_EX" != "--noexamplesbuilt" ]; then
        usage
    fi
fi
stop_etsi_mec_sandbox
sleep $TIMEOUT
build_etsi_mec_sandbox
docker image prune -f

exit 0
