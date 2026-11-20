#!/bin/bash

#set -e
set -vx

################# Functions ###########################
# Do alignment for AdvantEDGE update
function advantedge_aligner() {
    CUR_DIR=`pwd`
    cd $CUR_DIR/$OUT/$CMD_LANG_SERVER_GOLANG
    for dir in $PROJECTS
    do
        cd $CUR_DIR/$OUT/$CMD_LANG_SERVER_GOLANG/$dir
        REP=`find . -type d -name go`
        if [ -d $REP ]
        then
            cd $REP
            mv go server
            cd server
            for f in `ls *.go`
            do
                sed --in-place 's/package swagger/package server/g' $f
            done
            cd -
        fi
        done

    cd $CUR_DIR/$OUT/$CMD_LANG_CLIENT_GOLANG
    for dir in $PROJECTS
    do
        cd $CUR_DIR/$OUT/$CMD_LANG_CLIENT_GOLANG/$dir
        find . -name "*.go" -exec sed --in-place 's/package swagger/package client/g' {} \;
    done
}

# Generate client/server stubs
function generate_stub() {
    # Process all MEC APIs
    for dir in $PROJECTS
    do
        # Update folder
        cd $dir
        git checkout .
        git pull --rebase=true
        cd -
        # Acquire full YAML file name
        IN_FILE=`ls ${dir}/*.yaml`
        for f in $IN_FILE
        do
            # Downgrade file
            downgrade_open_api_file $f
            $JAVA_CMD --api-package ${JAVA_PACKAGE_SERVER} -l $CMD_LANG_SERVER_GOLANG -i ./$f -o $OUT/$CMD_LANG_SERVER_GOLANG/$dir/`basename $f .yaml`
            $JAVA_CMD --api-package ${JAVA_PACKAGE_CLIENT_GOLANG} -l $CMD_LANG_CLIENT_GOLANG -i ./$f -o $OUT/$CMD_LANG_CLIENT_GOLANG/$dir/`basename $f .yaml`
            $JAVA_CMD --api-package ${JAVA_PACKAGE_CLIENT_PYTHON} -l $CMD_LANG_CLIENT_PYTHON -i ./$f -o $OUT/$CMD_LANG_CLIENT_PYTHON/$dir/`basename $f .yaml`
        done
    done

    # Set output rights
    chown -R 1000:1000 $OUT
}

# Downgrade OpenAPI file
function downgrade_open_api_file() {
    PARAM=$1
    sed --in-place 's/3\.1\.0/3\.0\.0/g' $PARAM
    sed --in-place 's/examples/example/g' $PARAM
    sed --in-place '/contentMediaType/d' $PARAM
    sed --in-place '/jsonSchemaDialect/d' $PARAM
    sed --in-place '/contentEncoding/d' $PARAM
    # FIXME Issue with meep-sandbox-api
    if [ `basename $PARAM yaml` != "API_driven_swagger_v5." ]
    then
        sed --in-place 's/localhost/localhost\/sandboxname/g' $PARAM
        sed --in-place 's/127\.0\.0\.1:8081/localhost\/sandboxname/g' $PARAM
    fi
}

################# Starting point ###########################
clear

# Install git not present in the gizmotronic/oracle-java8 image
apt-get update
apt install -y git

# Setup variables
PROJECTS=`ls -d gs*`
JAVA_CMD='java -jar ./swagger-codegen-cli-3.0.29.jar generate'
TEMPLATE_PATH=./swagger-templates/templates
CMD_TEMPLATE_SERVER=go-server
CMD_TEMPLATE_CLIENT_GOLANG=go
CMD_TEMPLATE_CLIENT_PYTHON=python
JAVA_PACKAGE_SERVER=${TEMPLATE_PATH}/${CMD_TEMPLATE_SERVER}/
JAVA_PACKAGE_CLIENT_GOLANG=${TEMPLATE_PATH}/${CMD_TEMPLATE_CLIENT_GOLANG}/
JAVA_PACKAGE_CLIENT_PYTHON=${TEMPLATE_PATH}/${CMD_TEMPLATE_CLIENT_PYTHON}/
CMD_LANG_SERVER_GOLANG=go-server
CMD_LANG_CLIENT_GOLANG=go
CMD_LANG_CLIENT_PYTHON=python
OUT=./out

# Create empty output folder
if [ -d $OUT ]
then
    rm -fr $OUT
fi
mkdir -p $OUT

generate_stub

advantedge_aligner

exit 0
