from pyinfra import host
from pyinfra.operations import files, server, systemd
from pyinfra.facts.server import Command

disable_swap = host.data.get('disable_swap', True)

if disable_swap:
    # Disable swap at runtime if enabled.
    # We check if there's any swap configured first.
    swap_total = host.get_fact(Command, "free -m | awk '/Swap/ {print $2}'")
    
    if swap_total and swap_total.strip() != "0":
        server.shell(
            name="Disable swap at runtime if enabled",
            commands=["swapoff -a"],
            _sudo=True
        )

    # Comment out any active swap entries in fstab
    files.replace(
        name="Comment out any active swap entries in fstab",
        path="/etc/fstab",
        text=r'^([^#].*\s+swap\s+.*)$',
        replace=r'# \1',
        _sudo=True
    )

# Ensure kernel modules are present
modules = ["overlay", "br_netfilter"]
for mod in modules:
    server.modprobe(
        name=f"Ensure kernel module {mod} is present",
        module=mod,
        present=True,
        _sudo=True
    )

# Persist kernel modules
files.template(
    name="Persist kernel modules",
    src="templates/k8s.conf.j2", # We will create a template or just write a file
    dest="/etc/modules-load.d/k8s.conf",
    mode="0644",
    _sudo=True
)


# Configure sysctl for Kubernetes networking
sysctl_vars = [
    ("net.bridge.bridge-nf-call-iptables", 1),
    ("net.bridge.bridge-nf-call-ip6tables", 1),
    ("net.ipv4.ip_forward", 1),
    ("net.ipv6.conf.all.disable_ipv6", 1),
    ("net.ipv6.conf.default.disable_ipv6", 1),
]

for name, value in sysctl_vars:
    server.sysctl(
        name=f"Configure sysctl {name}",
        key=name,
        value=value,
        persist=True,
        _sudo=True
    )

# Reload systemd (if needed)
systemd.daemon_reload(
    name="Reload systemd",
    _sudo=True
)
