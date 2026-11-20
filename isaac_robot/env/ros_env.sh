# Shell for Nav2, IPE and demo scripts.  Usage:  source env/ros_env.sh
# Do NOT source the ROS 2 tarball setup.bash in this shell (libraries get mixed).

CONDA_DIR="${CONDA_DIR:-$HOME/miniforge3}"
source "$CONDA_DIR/bin/activate" ros_nav

export RMW_IMPLEMENTATION=rmw_fastrtps_cpp   # same RMW as Isaac (run_isaac.sh)
# Each robot is one ROS domain: prefix commands with ROS_DOMAIN_ID=<n>