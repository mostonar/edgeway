from pyinfra import host
from pyinfra.operations import server, files
from lib.operations import kubernetes

# Upload join command to worker
files.put(
    name="Upload join command to worker",
    src="/tmp/kubeadm_join.sh",
    dest="/tmp/kubeadm_join.sh",
    mode="0755",
    _sudo=True
)

# Join the cluster
kubernetes.join_cluster(
    name="Join worker to cluster",
    join_command_path="/tmp/kubeadm_join.sh",
    _sudo=True
)
