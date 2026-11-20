#!/bin/sh -e

set -vx

DESC="AdvantEDGE platform"
PLTF="Unknown"
TIMEOUT=7
IS_RUNNING=`kubectl get pods --all-namespaces | grep "meep-virt-engine" | awk '{print $2}'`
ADV_PATH=~/AdvantEDGE

start_advantedge() {
    if [ -z "$IS_RUNNING" ] ; then
        meepctl deploy dep all
        sleep $TIMEOUT
        meepctl deploy core all
        sleep $TIMEOUT
        kubectl get pods --all-namespaces
        sleep $TIMEOUT
	IS_RUNNING=`kubectl get pods --all-namespaces | grep "meep-virt-engine" | awk '{print $2}'`
    fi
}

stop_avdantedge() {
    if [ -n "$IS_RUNNING" ]; then
        meepctl delete core
        sleep $TIMEOUT
        meepctl delete dep
        sleep $TIMEOUT
        kubectl get pods --all-namespaces
        sleep $TIMEOUT
        #helm delete --no-hooks meep-prometheus
	IS_RUNNING=`kubectl get pods --all-namespaces | grep "meep-virt-engine" | awk '{print $2}'`
   fi
}

switch_to_advantedge_pltf() {
    check_pltf_mode
    if [ "$PLTF" = "AdvantEDGE" ]; then
        start_advantedge
        return
    fi
    stop_avdantedge

    rm -fr charts/observability/grafana/dashboards/mec-sandbox.json config/api/ config/permissions.yaml
    cp ./.meepctl-repocfg_adv.yaml ./.meepctl-repocfg.yaml
    git checkout --force config/objstore-thanos-archive.yaml config/objstore-thanos.yaml config/secrets.yaml examples/demo4-ue/src/demo-server/entrypoint.sh
    rm ~/.meep && ln -sf ~/.meep_advantedge ~/.meep

    build_advantedge
    start_advantedge
}

switch_to_sandbox_pltf() {
    check_backup
    if [ "$PLTF" = "Sandbox" ]; then
        start_advantedge
        return
    fi

    stop_avdantedge
    rm ~/.meep && ln -sf ~/.meep_sandbox ~/.meep
    cd ../mec-sandbox
    ./build.sh
    ./deploy.sh
    cd -
    python3 ~/AdvantEDGE/config/configure-secrets.py set ~/AdvantEDGE/config/secrets.yaml
    build_advantedge
    start_advantedge
}

check_pltf_mode() {
    if [ -f charts/observability/grafana/dashboards/mec-sandbox.json ]; then
        # Sandbox mode
        PLTF="Sandbox"
    else
        FT=`grep "frontend: false" .meepctl-repocfg.yaml`
        if [ -n "$FT" ]; then
            # AdvantEDGE mode
            PLTF="AdvantEDGE"
        fi
        unset FT
    fi
}

check_backup() {
    check_pltf_mode
    if [ "$PLTF" = "AdvantEDGE" ]; then
        if [ -f .meepctl-repocfg_adv.yaml ]; then
            echo "$0: AdvantEDGE backup already done"
        else
            echo "$0: AdvantEDGE backup created"
            cp .meepctl-repocfg.yaml .meepctl-repocfg_adv.yaml
        fi
    else
        if [ "$PLTF" = "Sandbox" ]; then
            if [ -f .meepctl-repocfg_adv.yaml ]; then
                echo "$0: AdvantEDGE backup already done"
            else
                echo "$0: Wrong configuration. Please, set up AdvantEDGE plateform"
                exit -1
            fi
        else
            echo "$0: Unknown configuration. Please, set up AdvantEDGE plateform"
            exit -1
        fi
    fi
}

build_advantedge() {
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
}

check_backup

case "$1" in
AdvantEDGE)
  echo "Switching to AdvantEDGE"
  switch_to_advantedge_pltf
  ;;
Sandbox)
  echo "Switching to Sandbox"
  switch_to_sandbox_pltf
  ;;
status)
  check_pltf_mode
  echo "Plateform: $PLTF"
  kubectl get pods -n default
  ;;
*)
  echo "Usage: $0 Switch between AdvantEDGE plateform and MEC Sandbox plateform" >&2
  echo "       $0 {AdvantEDGE|Sandbox|status}" >&2
  exit 1
  ;;
esac

exit 0
