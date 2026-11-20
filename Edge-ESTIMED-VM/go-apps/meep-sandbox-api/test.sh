#!/bin/bash

set -e
set -vx

TIMEOUT=5
CURL_VERBOSE="" #--verbose

function logout() {
    RES=`curl $CURL_VERBOSE --request POST https://mec-platform.etsi.org/sandbox-api/v1/logout?sandbox_name=$SANDBOX_NAME --header "Accept: application/json"`
    if [ $? != 0 ]
    then
        echo "Failed to logout from the Sandbox"
        exit -1
    fi
}

echo "1. ========================================================================"
RES=`curl https://mec-platform.etsi.org/sandbox-api/v1/ --header "Accept: application/json"`
if [ "$RES" != "Hello World!" ]
then
    echo "Failed to access Sandbox"
    exit -1
fi
echo "Step1: $RES"

echo "2. ========================================================================"
RES=`curl $CURL_VERBOSE --request POST https://mec-platform.etsi.org/sandbox-api/v1/login?provider=gitlab --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    exit -1
fi
echo "Step2: $RES"

sleep $TIMEOUT

#str="Paris, France, Europe, Los Angeles"; IFS=', ' read -r -a array <<< "$str"
SANDBOX_NAME=`kubectl get pods -A | grep 'meep-sandbox-ctrl-' | awk '{print $1}'`
echo "Step2.1: $SANDBOX_NAME"

kubectl get pods -A

echo "3. ========================================================================"
RES=`curl $CURL_VERBOSE https://mec-platform.etsi.org/sandbox-api/v1/sandboxNetworkScenarios --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    logout
    exit -1
fi
echo "Step3: $RES"

sleep $TIMEOUT

echo "4. ========================================================================"
RES=`curl $CURL_VERBOSE https://mec-platform.etsi.org/sandbox-api/v1/sandboxNetworkScenarios/4g-5g-macro-v2x --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    logout
    exit -1
fi
echo "Step4: $RES"

sleep $TIMEOUT

echo "5. ========================================================================"
RES=`curl $CURL_VERBOSE --request POST https://mec-platform.etsi.org/sandbox-api/v1/sandboxNetworkScenarios/4g-5g-macro-v2x?sandbox_name=$SANDBOX_NAME --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    logout
    exit -1
fi
echo "Step5: $RES"

kubectl get pods -A

SANDBOX_CTRL=`kubectl get pods -A | grep 'meep-sandbox-ctrl-' | awk '{print $2}'`
echo "Step5.1: $SANDBOX_CTRL"
kubectl logs -n $SANDBOX_NAME $SANDBOX_CTRL > ~/AdvantEDGE/meep-sandbox-ctrl.log 2>&1

sleep $TIMEOUT

echo "6. ========================================================================"
RES=`curl $CURL_VERBOSE https://mec-platform.etsi.org/sandbox-api/v1/sandboxMecServices?sandbox_name=$SANDBOX_NAME --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    logout
    exit -1
fi
echo "Step6: $RES"

sleep $TIMEOUT

echo "7. ========================================================================"
RES=`curl $CURL_VERBOSE https://mec-platform.etsi.org/sandbox-api/v1//sandbox-api/v1/sandboxAppInstances?sandbox_name=$SANDBOX_NAME --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    logout
    exit -1
fi

sleep $TIMEOUT

echo "8. ========================================================================"
RES=`curl $CURL_VERBOSE --request DELETE https://mec-platform.etsi.org/sandbox-api/v1/sandboxNetworkScenarios/4g-5g-macro-v2x?sandbox_name=$SANDBOX_NAME --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    logout
    exit -1
fi
echo "Step7: $RES"

sleep $TIMEOUT

echo "9. ========================================================================"
RES=`curl $CURL_VERBOSE --request POST https://mec-platform.etsi.org/sandbox-api/v1/logout?sandbox_name=$SANDBOX_NAME --header "Accept: application/json"`
if [ $? != 0 ]
then
    echo "Failed to access Sandbox"
    exit -1
fi
echo "Step8: $RES"

sleep $TIMEOUT

SANDBOX_POD=`kubectl get pods -A | grep 'meep-sandbox-api-' | awk '{print $2}'`
kubectl logs $SANDBOX_POD > ~/AdvantEDGE/meep-sandbox-api.log 2>&1

sleep $TIMEOUT

ekubectl get pods -A

exit 0
