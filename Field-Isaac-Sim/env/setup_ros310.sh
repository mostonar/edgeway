#!/bin/bash
# Create the ros310 env: a library store for the Isaac Sim ROS 2 bridge (not used to run anything).
# The bridge needs old-SONAME libs (spdlog 1.9.2, libtinyxml2.so.9).
# conda installs tinyxml2 11, so the .so.9 name is added as a symlink at the end.
set -e

CONDA_DIR="${CONDA_DIR:-$HOME/miniforge3}"
HERE="$(cd "$(dirname "$0")" && pwd)"

source "$CONDA_DIR/bin/activate"
conda create -y -n ros310 --file "$HERE/ros310.txt"

LIB="$CONDA_DIR/envs/ros310/lib"
ln -sf libtinyxml2.so "$LIB/libtinyxml2.so.9"   # -> libtinyxml2.so -> .so.11
ls -l "$LIB"/libtinyxml2.so* "$LIB"/libspdlog.so*