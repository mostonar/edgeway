from pyinfra import local
from pyinfra import host

# Load Pyinfra tasks

# Kubernetes Master Setup
if "k8s_masters" in host.groups:
    # System Configuration
    local.include("tasks/system/common.py")
    local.include("tasks/system/kernel.py")
    # Container Runtime
    local.include("tasks/container_runtime/docker.py")
    local.include("tasks/container_runtime/containerd.py")

    # Kubernetes Cluster (Common packages)
    local.include("tasks/k8s_cluster/kubernetes_common.py")
    local.include("tasks/k8s_cluster/kubernetes_master.py")
    local.include("tasks/k8s_cluster/cni_calico.py")
    local.include("tasks/k8s_cluster/helm.py")

    # Dev Environment & Sandbox
    install_dev_env = host.data.get('install_dev_env', True)
    install_mec_sandbox = host.data.get('install_mec_sandbox', True)
    if install_dev_env:
        local.include("tasks/apps/dev_env.py")
    if install_mec_sandbox: 
        local.include("tasks/apps/mec_sandbox.py")

# Kubernetes Worker Setup
if "k8s_workers" in host.groups:
    # System Configuration
    local.include("tasks/system/common.py")
    local.include("tasks/system/kernel.py")
    # Container Runtime
    local.include("tasks/container_runtime/docker.py")
    local.include("tasks/container_runtime/containerd.py")

    # Kubernetes Cluster (Common packages)
    local.include("tasks/k8s_cluster/kubernetes_common.py")
    local.include("tasks/k8s_cluster/kubernetes_worker.py")