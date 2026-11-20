from pyinfra import host
from pyinfra.operations import server, files, apt
from pyinfra.facts.server import Command

docker_gpg_key_url = host.data.get('docker_gpg_key_url')
docker_repo_arch = host.data.get('docker_repo_arch')
docker_repo_url = host.data.get('docker_repo_url')
docker_repo_codename = host.data.get('docker_repo_codename')
docker_repo_component = host.data.get('docker_repo_component')
target_user = host.data.get('target_user')

# Remove any conflicting/stale Docker repository files or old GPG keys from previous installations
server.shell(
    name="Remove stale or conflicting Docker repository files and keys",
    commands=[
        "rm -f /etc/apt/sources.list.d/*docker* /etc/apt/keyrings/docker.gpg /usr/share/keyrings/docker*.gpg",
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

# Ensure apt keyrings directory exists
files.directory(
    name="Ensure apt keyrings directory exists",
    path="/etc/apt/keyrings",
    mode="0755",
    present=True,
    _sudo=True
)

# Download Docker GPG key
files.download(
    name="Download Docker GPG key file",
    src=docker_gpg_key_url,
    dest="/etc/apt/keyrings/docker.asc",
    mode="0644",
    _sudo=True
)

# Add Docker repository
apt.repo(
    name="Add Docker repository",
    src=f"deb [arch={docker_repo_arch} signed-by=/etc/apt/keyrings/docker.asc] {docker_repo_url} {docker_repo_codename} {docker_repo_component}",
    filename="docker",
    present=True,
    _sudo=True
)

# Install Docker engine and components
apt.packages(
    name="Install Docker engine and components",
    packages=["docker-ce", "docker-ce-cli", "docker-compose-plugin"],
    present=True,
    update=True,
    _sudo=True
)

# Hold Docker packages
for pkg in ["docker-ce", "docker-ce-cli", "docker-compose-plugin"]:
    pkg_status = host.get_fact(Command, f"dpkg-query -W -f='${{Status}}' {pkg} || true")
    if "hold" not in (pkg_status or ""):
        server.shell(
            name=f"Hold {pkg}",
            commands=[f"apt-mark hold {pkg}"],
            _sudo=True
        )

# Add user to Docker group
server.group(
    name="Ensure docker group exists",
    group="docker",
    present=True,
    _sudo=True
)

server.user(
    name="Add user to Docker group",
    user=target_user,
    groups=["docker"],
    append=True,
    _sudo=True
)

# Ensure docker socket is group-accessible
socket_stat = host.get_fact(Command, "stat -c '%G:%a' /var/run/docker.sock 2>/dev/null || echo missing")
if socket_stat != "missing" and socket_stat != "docker:660":
    server.shell(
        name="Ensure docker socket is group-accessible",
        commands=[
            "chgrp docker /var/run/docker.sock && chmod 0660 /var/run/docker.sock"
        ],
        _sudo=True
    )

# Stop Docker from setting the iptables FORWARD policy to DROP on every start.
# That policy also drops Kubernetes pod egress (Calico's own accept rules live
# in a separate nftables table, and a drop in any table wins), so after any
# Docker restart (e.g. meepctl's registry cert trust step) pods lose internet
# access and CoreDNS upstream lookups time out ("server misbehaving").
# Merges the key into an existing daemon.json and restarts Docker only when it changed.
server.shell(
    name="Configure Docker daemon to not drop forwarded (pod) traffic",
    commands=[
        "python3 - <<'EOF'\n"
        "import json, os, subprocess\n"
        "path = '/etc/docker/daemon.json'\n"
        "cfg = json.load(open(path)) if os.path.exists(path) and os.path.getsize(path) else {}\n"
        "if cfg.get('ip-forward-no-drop') is not True:\n"
        "    cfg['ip-forward-no-drop'] = True\n"
        "    os.makedirs('/etc/docker', exist_ok=True)\n"
        "    json.dump(cfg, open(path, 'w'), indent=2)\n"
        "    subprocess.run(['systemctl', 'restart', 'docker'], check=True)\n"
        "EOF",
        # Undo a DROP policy already set by an earlier Docker start
        "iptables -P FORWARD ACCEPT",
    ],
    _sudo=True,
)

# Configure Docker MTU to 1450 to match Calico overlay and prevent DNS timeouts
# server.shell(
#     name="Configure Docker daemon.json with MTU 1450",
#     commands=[
#         "grep -q '\"mtu\": 1450' /etc/docker/daemon.json 2>/dev/null || (echo '{\"mtu\": 1450}' > /etc/docker/daemon.json && systemctl restart docker)"
#     ],
#     _sudo=True
# )
