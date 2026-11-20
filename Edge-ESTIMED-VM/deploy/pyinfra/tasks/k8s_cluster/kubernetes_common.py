from pyinfra import host
from pyinfra.operations import server, files, apt
from pyinfra.facts.server import Command

kubernetes_version = host.data.get('kubernetes_version')
kubernetes_repo_apt_key_url = host.data.get('kubernetes_repo_apt_key_url')
kubernetes_repo_apt_entry = host.data.get('kubernetes_repo_apt_entry')
pod_network_cidr = host.data.get('pod_network_cidr')
target_user = host.data.get('target_user')
target_home = host.data.get('target_home')

version_string = kubernetes_version.replace('v', '')

# Install Kubeadm dependencies
apt.packages(
    name="Install Kubeadm dependencies",
    packages=["apt-transport-https", "ca-certificates", "curl", "gpg"],
    present=True,
    update=True,
    _sudo=True
)

# Remove any invalid binary .gpg file if present from previous runs
files.file(
    name="Remove any stale binary kubernetes GPG keyring",
    path="/etc/apt/keyrings/kubernetes-apt-keyring.gpg",
    present=False,
    _sudo=True
)

# Import Kubernetes GPG key as ASCII armored (.asc)
files.download(
    name="Import Kubernetes GPG key",
    src=kubernetes_repo_apt_key_url,
    dest="/etc/apt/keyrings/kubernetes-apt-keyring.asc",
    mode="0644",
    _sudo=True
)


# Add Kubernetes apt repository
apt.repo(
    name="Add Kubernetes apt repository",
    src=kubernetes_repo_apt_entry,
    filename="kubernetes",
    present=True,
    _sudo=True
)

# Install kube packages
packages = [
    f"kubelet={version_string}-*",
    f"kubeadm={version_string}-*",
    f"kubectl={version_string}-*"
]

apt.packages(
    name="Install kube packages (kubeadm, kubelet, kubectl)",
    packages=packages,
    present=True,
    update=True,
    _sudo=True
)

# Hold kube packages
for pkg in ["kubelet", "kubeadm", "kubectl"]:
    pkg_status = host.get_fact(Command, f"dpkg-query -W -f='${{Status}}' {pkg} || true")
    if "hold" not in (pkg_status or ""):
        server.shell(
            name=f"Hold {pkg}",
            commands=[f"apt-mark hold {pkg}"],
            _sudo=True
        )


