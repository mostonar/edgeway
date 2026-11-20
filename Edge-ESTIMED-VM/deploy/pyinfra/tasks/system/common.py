from pyinfra import host
from pyinfra.operations import apt, systemd, files, server

apt_base_packages = host.data.get('apt_base_packages', [])

# Recover from any previously interrupted dpkg operations
server.shell(
    name="Ensure dpkg is in a clean configured state",
    commands=["dpkg --configure -a || true"],
    _sudo=True
)

# Remove any conflicting/stale Docker or Kubernetes apt sources and keys before initial apt update
server.shell(
    name="Remove stale or conflicting repository files before apt update",
    commands=[
        "rm -f /etc/apt/sources.list.d/*docker* /etc/apt/sources.list.d/*kubernetes* /etc/apt/keyrings/docker.gpg /etc/apt/keyrings/docker.asc /usr/share/keyrings/docker*.gpg",
    ],
    _sudo=True,
)

files.line(
    name="Remove docker from sources.list",
    path="/etc/apt/sources.list",
    line=r".*download\.docker\.com.*",
    present=False,
    _sudo=True,
)

files.line(
    name="Remove kubernetes from sources.list",
    path="/etc/apt/sources.list",
    line=r".*pkgs\.k8s\.io.*",
    present=False,
    _sudo=True,
)

# Update apt cache and install base packages
apt.packages(
    name="Update apt cache and install base packages",
    packages=apt_base_packages,
    update=True,
    _sudo=True
)


# Stop unattended-upgrades temporarily (to avoid apt lock)
systemd.service(
    name="Stop unattended-upgrades temporarily",
    service="unattended-upgrades",
    running=False,
    _sudo=True,
    _ignore_errors=True # Ansible had 'failed_when: not-found not in result', pyinfra ignores errors here for simplicity if not found.
)

# Ensure /etc/apt/keyrings exists
files.directory(
    name="Ensure /etc/apt/keyrings exists",
    path="/etc/apt/keyrings",
    mode="0755",
    present=True,
    _sudo=True
)
