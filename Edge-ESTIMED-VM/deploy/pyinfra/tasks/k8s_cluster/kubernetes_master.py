from pyinfra import host
from pyinfra.operations import server, files
from lib.operations import kubernetes

pod_network_cidr = host.data.get('pod_network_cidr')
target_user = host.data.get('target_user')
target_home = host.data.get('target_home')

# If it's a first run, init kubernetes
kubernetes.init_control_plane(
    name="Initialize Kubernetes control plane",
    pod_network_cidr=pod_network_cidr,
    _sudo=True
)

# Create .kube directory for user
files.directory(
    name="Create .kube directory for user",
    path=f"{target_home}/.kube",
    mode="0700",
    present=True
)

# Copy admin.conf to user kubeconfig
server.shell(
    name="Copy admin.conf to user kubeconfig",
    commands=[
        f"cp /etc/kubernetes/admin.conf {target_home}/.kube/config",
        f"chown {target_user}:{target_user} {target_home}/.kube/config",
        f"chmod 0600 {target_home}/.kube/config"
    ],
    _sudo=True
)

from pyinfra.facts.files import File

# Optimization: Prevent token sprawl and redundant join command generation.
# Why: On standalone control planes without worker nodes, generating kubeadm join tokens
# creates unnecessary cluster secrets and temporary files on the local executor.
if "k8s_workers" in host.groups and len(host.groups["k8s_workers"]) > 0:
    if not host.get_fact(File, path="/tmp/kubeadm_join.sh"):
        server.shell(
            name="Get kubeadm join command",
            commands=["kubeadm token create --print-join-command > /tmp/kubeadm_join.sh"],
            _sudo=True
        )

    files.download(
        name="Fetch join command to local",
        src="/tmp/kubeadm_join.sh",
        dest="/tmp/kubeadm_join.sh",
        _sudo=True
    )
