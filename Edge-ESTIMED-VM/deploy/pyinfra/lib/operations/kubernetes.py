import shlex

from pyinfra import host
from pyinfra.api import FactBase, operation, OperationError, StringCommand
from pyinfra.facts.files import File

def _get_kubeconfig_env(kubeconfig):
    return f"KUBECONFIG={kubeconfig} " if kubeconfig else ""

class ConfigMap(FactBase):
    """
    Gets the YAML of a ConfigMap.
    Resiliency Design: During Phase 1 fact gathering on uninitialized clusters or `--dry` runs,
    kubectl may not be installed yet. Declaring `requires_command` and returning None in `default`
    prevents PyInfra from aborting DAG generation with MissingCommandError.
    """
    requires_command = "kubectl"

    def default(self):
        return None

    def command(self, configmap_name, namespace, kubeconfig=None):
        env_str = _get_kubeconfig_env(kubeconfig)
        return f"{env_str}kubectl get configmap {configmap_name} -n {namespace} -o yaml || true"


class ReachableDnsServers(FactBase):
    """
    Returns the subset of the given DNS servers that answer a plain DNS query
    (UDP/53) from this host. Pod traffic leaves through the node, so this is
    what CoreDNS will be able to reach; some networks block specific public
    resolvers (e.g. 1.1.1.1 is often hijacked by routers/ISPs).
    """
    requires_command = "python3"

    def default(self):
        return []

    def command(self, servers):
        # No heredoc: pyinfra wraps the command in "(...)" for requires_command,
        # which would put ")" on the terminator line and break the heredoc
        script = (
            "import socket, struct, sys\n"
            "query = struct.pack('>HHHHHH', 0x4d45, 0x0100, 1, 0, 0, 0)\n"
            "query += b''.join(bytes([len(p)]) + p.encode() for p in 'github.com'.split('.'))\n"
            "query += b'\\0' + struct.pack('>HH', 1, 1)\n"
            "for server in sys.argv[1:]:\n"
            "    try:\n"
            "        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)\n"
            "        s.settimeout(2)\n"
            "        s.sendto(query, (server, 53))\n"
            "        if s.recv(512)[:2] == query[:2]:\n"
            "            print(server)\n"
            "    except OSError:\n"
            "        pass\n"
        )
        return "python3 -c " + shlex.quote(script) + " " + " ".join(shlex.quote(s) for s in servers)

    def process(self, output):
        return [line.strip() for line in output if line.strip()]


@operation()
def apply(manifest_path, kubeconfig=None, server_side=False, wait_resource=None, wait_condition="Available", wait_namespace=None, wait_timeout="300s"):
    """
    Apply a Kubernetes manifest.
    `kubectl apply` is inherently idempotent.
    """
    env_str = _get_kubeconfig_env(kubeconfig)
    apply_cmd = f"{env_str}kubectl apply -f {manifest_path}"
    if server_side:
        apply_cmd += " --server-side"
    
    yield StringCommand(apply_cmd)

    if wait_resource:
        ns_flag = f"-n {wait_namespace}" if wait_namespace else ""
        yield StringCommand(
            f"{env_str}kubectl wait --for=condition={wait_condition} {wait_resource} {ns_flag} --timeout={wait_timeout}"
        )

@operation()
def wait_for_condition(resource, condition, namespace=None, timeout="300s", kubeconfig=None):
    """
    Wait for a specific condition on a Kubernetes resource.
    """
    env_str = _get_kubeconfig_env(kubeconfig)
    ns_flag = f"-n {namespace}" if namespace else ""
    yield StringCommand(
        f"{env_str}kubectl wait --for=condition={condition} {resource} {ns_flag} --timeout={timeout}"
    )

@operation()
def taint_nodes(taint_string, node_selector=None, kubeconfig=None, ignore_errors=False):
    """
    Taint or untaint Kubernetes nodes idempotently.
    e.g. "key=value:NoSchedule" to add, or "key-" to remove.
    """
    env_str = _get_kubeconfig_env(kubeconfig)
    ns_flag = f"-l {node_selector}" if node_selector and node_selector != "--all" else ""
    
    if taint_string.endswith('-'):
        # Untaint operation: only untaint nodes that actually have the taint
        key = taint_string[:-1]
        cmd = (
            f"for node in $({env_str}kubectl get nodes {ns_flag} -o jsonpath='{{.items[*].metadata.name}}'); do "
            f"if {env_str}kubectl get node $node -o jsonpath='{{.spec.taints[*].key}}' | grep -qw '{key}'; then "
            f"{env_str}kubectl taint nodes $node {taint_string}; "
            f"fi; "
            f"done"
        )
        yield StringCommand(cmd)

    else:
        # Taint operation: use --overwrite to make it idempotent
        taint_target = ns_flag if ns_flag else "--all"
        yield StringCommand(f"{env_str}kubectl taint nodes {taint_target} {taint_string} --overwrite")

@operation()
def patch_configmap(configmap_name, namespace, search_string, replace_string, rollout_restart=None, kubeconfig=None):
    """
    Idempotently replaces a string inside a ConfigMap and optionally restarts a deployment.
    """
    current_yaml = host.get_fact(ConfigMap, configmap_name=configmap_name, namespace=namespace, kubeconfig=kubeconfig)

    
    if current_yaml and replace_string in current_yaml:
        return # Already patched
        
    env_str = _get_kubeconfig_env(kubeconfig)
    cmd = (
        f"{env_str}kubectl get configmap {configmap_name} -n {namespace} -o yaml | "
        f"sed 's|{search_string}|{replace_string}|g' | "
        f"{env_str}kubectl apply -f -"
    )
    yield StringCommand(cmd)

    
    if rollout_restart:
        yield StringCommand(f"{env_str}kubectl rollout restart {rollout_restart} -n {namespace}")
        yield StringCommand(f"{env_str}kubectl rollout status {rollout_restart} -n {namespace} --timeout=60s")

@operation()
def configure_coredns_upstreams(servers, kubeconfig=None):
    """
    Points CoreDNS at the given upstream resolvers, keeping only those that
    actually answer from this node. Servers are tried strictly in the given
    order (`policy sequential`), so later ones are fallbacks, not load-shared.
    Idempotent: rewrites whatever `forward .` block exists (kubeadm's
    /etc/resolv.conf or a previous list) and leaves the ConfigMap alone when
    it already matches.
    """
    reachable = host.get_fact(ReachableDnsServers, servers=servers)
    unreachable = [s for s in servers if s not in reachable]
    if unreachable:
        host.noop(f"Skipping unreachable DNS servers: {', '.join(unreachable)}")
    if not reachable:
        raise OperationError(
            f"None of the DNS servers {servers} answer from this host; "
            "set CLUSTER_DNS_SERVERS in .env to resolvers reachable on this network"
        )

    forward = "forward . " + " ".join(reachable) + " {"
    block = forward + "\n       max_concurrent 1000\n       policy sequential\n    }"
    current_yaml = host.get_fact(ConfigMap, configmap_name="coredns", namespace="kube-system", kubeconfig=kubeconfig)
    if current_yaml and forward in current_yaml and "policy sequential" in current_yaml:
        return  # Already configured

    # Replace the whole forward block (header and options) on the node, so
    # re-runs with a different server list never duplicate options
    rewrite = (
        "import json, re, sys; "
        "cm = json.load(sys.stdin); "
        "cm['data']['Corefile'] = re.sub(r'forward \\. [^{]*\\{[^}]*\\}', lambda m: sys.argv[1], cm['data']['Corefile'], count=1); "
        "json.dump(cm, sys.stdout)"
    )
    env_str = _get_kubeconfig_env(kubeconfig)
    yield StringCommand(
        f"{env_str}kubectl get configmap coredns -n kube-system -o json | "
        f"python3 -c {shlex.quote(rewrite)} {shlex.quote(block)} | "
        f"{env_str}kubectl apply -f -"
    )
    yield StringCommand(f"{env_str}kubectl rollout restart deployment/coredns -n kube-system")
    yield StringCommand(f"{env_str}kubectl rollout status deployment/coredns -n kube-system --timeout=60s")

@operation()
def init_control_plane(pod_network_cidr):
    """
    Idempotently initialize the Kubernetes control plane using kubeadm.
    """
    has_admin_conf = host.get_fact(File, path="/etc/kubernetes/admin.conf")
    if has_admin_conf:
        return
    yield StringCommand(f"kubeadm init --pod-network-cidr={pod_network_cidr}")

@operation()
def join_cluster(join_command_path="/tmp/kubeadm_join.sh"):
    """
    Idempotently join a worker node to the Kubernetes cluster.
    """
    has_kubelet_conf = host.get_fact(File, path="/etc/kubernetes/kubelet.conf")
    if has_kubelet_conf:
        return
        
    yield StringCommand(f"sh {join_command_path}")
