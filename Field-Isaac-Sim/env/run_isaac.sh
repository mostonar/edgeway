#!/bin/bash
# Launch Isaac Sim (streaming) with the ROS 2 Humble bridge libraries.
# Change the three paths below (or set them as env vars) for your machine.

ISAACSIM_DIR="${ISAACSIM_DIR:-$HOME/isaacsim}"
CONDA_DIR="${CONDA_DIR:-$HOME/miniforge3}"
ROS_TARBALL_DIR="${ROS_TARBALL_DIR:-$HOME/ros2_humble/ros2-linux}"

# Order matters:
#   1) Isaac bundled Humble libs
#   2) conda env ros310 : old-SONAME libs the bridge needs (spdlog 1.9.2, libtinyxml2.so.9)
#   3) ROS 2 Humble tarball libs
# Do NOT `source` the tarball setup.bash here or in the ros_nav shell.
export LD_LIBRARY_PATH="$ISAACSIM_DIR/exts/isaacsim.ros2.core/humble/lib:$CONDA_DIR/envs/ros310/lib:$ROS_TARBALL_DIR/lib:$LD_LIBRARY_PATH"
export ROS_DISTRO=humble
export RMW_IMPLEMENTATION=rmw_fastrtps_cpp   # must match the ros_nav side

exec "$ISAACSIM_DIR/isaac-sim.streaming.sh" "$@"