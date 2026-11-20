from pyinfra import host
from pyinfra.operations import apt, server, files, systemd

containerd_version = host.data.get('containerd_version')
containerd_config_path = host.data.get('containerd_config_path')

pkg = f"containerd.io={containerd_version}" if containerd_version and containerd_version != "latest" else "containerd.io"

# Install containerd
apt.packages(
    name="Install containerd",
    packages=[pkg],
    present=True,
    update=True,
    cache_time=3600,
    _sudo=True
)


# Generate default containerd config
server.shell(
    name="Generate default containerd config",
    commands=[f"containerd config default > {containerd_config_path}"],
    _sudo=True
)

# Ensure SystemdCgroup is true
files.replace(
    name="Ensure SystemdCgroup is true",
    path=containerd_config_path,
    text=r'SystemdCgroup = false',
    replace='SystemdCgroup = true',
    _sudo=True
)

# Replace containerd sandbox image
files.replace(
    name="Replace containerd sandbox image",
    path=containerd_config_path,
    text=r'sandbox_image = "registry.k8s.io/pause:3.8',
    replace='sandbox_image = "registry.k8s.io/pause:3.10',
    _sudo=True
)

# Enable certs.d config_path
files.replace(
    name="Enable certs.d config_path for registry mirrors",
    path=containerd_config_path,
    text=r'config_path = ""',
    replace='config_path = "/etc/containerd/certs.d"',
    _sudo=True
)

files.directory(
    name="Create containerd certs.d directory for registry.k8s.io",
    path="/etc/containerd/certs.d/registry.k8s.io",
    _sudo=True,
)

files.template(
    name="Configure explicit IPv4 mirror for registry.k8s.io",
    src="templates/registry.k8s.io.hosts.toml.j2",
    dest="/etc/containerd/certs.d/registry.k8s.io/hosts.toml",
    _sudo=True,
)


# Restart containerd
systemd.service(
    name="Restart containerd",
    service="containerd",
    running=True,
    restarted=True,
    _sudo=True
)
