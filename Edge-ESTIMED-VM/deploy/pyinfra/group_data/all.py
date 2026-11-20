import os as __os
from lib.config_helpers import (
    get_target_user_and_home as __get_target_user_and_home,
    get_mec_host_address as __get_mec_host_address,
    get_oauth_config as __get_oauth_config,
)

# =============================================================================
# PYINFRA GROUP DATA (ALL HOSTS)
# Declarative inventory variables for the ETSI MEC Sandbox deployment.
# Procedural validation and interactive prompts are handled in lib.config_helpers.
# =============================================================================

# -----------------------------------------------------------------------------
# Target Environment
# -----------------------------------------------------------------------------
target_user, target_home = __get_target_user_and_home()

# -----------------------------------------------------------------------------
# MEC Sandbox Host & Directories
# -----------------------------------------------------------------------------
mec_host_address = __get_mec_host_address()
mec_sandbox_dir = __os.environ.get("MEC_SANDBOX_DIR", f"{target_home}/etsi-mec-sandbox")
mec_frontend_dir = __os.environ.get("MEC_FRONTEND_DIR", f"{mec_sandbox_dir}/etsi-mec-sandbox-frontend")

# -----------------------------------------------------------------------------
# Cluster DNS
# -----------------------------------------------------------------------------
# Upstream resolvers for CoreDNS, in priority order: Cloudflare first, Google
# as fallback. Unreachable ones are skipped (some routers/ISPs block 1.1.1.1).
cluster_dns_servers = (__os.environ.get("CLUSTER_DNS_SERVERS") or "1.1.1.1 1.0.0.1 8.8.8.8 8.8.4.4").replace(",", " ").split()

# -----------------------------------------------------------------------------
# OAuth Configuration (GitHub & GitLab)
# -----------------------------------------------------------------------------
__oauth = __get_oauth_config()
github_enabled = __oauth["github_enabled"]
gitlab_enabled = __oauth["gitlab_enabled"]
github_client_id = __oauth["github_client_id"]
github_client_secret = __oauth["github_client_secret"]
gitlab_client_id = __oauth["gitlab_client_id"]
gitlab_client_secret = __oauth["gitlab_client_secret"]
oauth_configured_providers = __oauth["configured_providers"]

# -----------------------------------------------------------------------------
# Base System Configuration
# -----------------------------------------------------------------------------
apt_base_packages = [
    "ca-certificates",
    "curl",
    "gnupg",
    "lsb-release",
    "software-properties-common",
    "git",
    "unzip",
    "tar",
    "python3",
    "python3-pip",
    "python3-yaml",
    "python3-ruamel.yaml",
    "acl",
]

disable_swap = True

# -----------------------------------------------------------------------------
# Container Runtime (Docker & containerd)
# -----------------------------------------------------------------------------
docker_package_state = "present"
containerd_version = "latest"

containerd_config_path = "/etc/containerd/config.toml"

docker_gpg_key_url = "https://download.docker.com/linux/ubuntu/gpg"
docker_gpg_key_path = "/usr/share/keyrings/docker-archive-keyring.gpg"
docker_repo_list_path = "/etc/apt/sources.list.d/docker.list"
docker_repo_url = "https://download.docker.com/linux/ubuntu"
docker_repo_component = "stable"

docker_repo_arch = "amd64"
docker_repo_codename = "jammy"

# -----------------------------------------------------------------------------
# Kubernetes Configuration
# -----------------------------------------------------------------------------
kubernetes_version = "v1.35.1"
kubernetes_version_series = "v1.35"
kubernetes_repo_apt_key_url = f"https://pkgs.k8s.io/core:/stable:/{kubernetes_version_series}/deb/Release.key"
kubernetes_repo_apt_entry = f"deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.asc] https://pkgs.k8s.io/core:/stable:/{kubernetes_version_series}/deb/ /"

kubeadm_cluster_name = "mec-sandbox"
# Pod CIDR is used for both `kubeadm init --pod-network-cidr` and the Calico IP pool.
# Must not overlap with node/LAN/VPN networks (192.168.0.0/16 is our VPN pool) or service_cidr.
pod_network_cidr = "10.244.0.0/16"
service_cidr = "10.96.0.0/12"
apiserver_advertise_address = "127.0.0.1"

# -----------------------------------------------------------------------------
# CNI (Calico) & Helm
# -----------------------------------------------------------------------------
calico_version = "v3.31.4"
calico_operator_crds_manifest = f"https://raw.githubusercontent.com/projectcalico/calico/{calico_version}/manifests/operator-crds.yaml"
calico_operator_manifest = f"https://raw.githubusercontent.com/projectcalico/calico/{calico_version}/manifests/tigera-operator.yaml"
# Rendered from templates/calico-custom-resources-bpf.yaml.j2 (upstream custom-resources-bpf.yaml
# with the IP pool CIDR set from pod_network_cidr) and applied from this path on the master.
calico_custom_resources_manifest = "/etc/kubernetes/addons/calico-custom-resources.yaml"

helm_version = "v3.14.4"

# -----------------------------------------------------------------------------
# Development Environment
# -----------------------------------------------------------------------------
install_dev_env = True
go_version = "1.25.0"
go_tar = f"go{go_version}.linux-amd64.tar.gz"
go_url = f"https://go.dev/dl/go{go_version}.linux-amd64.tar.gz"
node_major = 24
node_version = "24.18.0"
npm_version = "12.0.1"
eslint_version = "9.39.5"
python_packages = ["pyyaml"]

# -----------------------------------------------------------------------------
# Registry & CA Trust
# -----------------------------------------------------------------------------
docker_registry_host = "meep-docker-registry"
docker_insecure_registries = []
docker_registry_mirrors = []
trust_k8s_ca_for_runtime = True
