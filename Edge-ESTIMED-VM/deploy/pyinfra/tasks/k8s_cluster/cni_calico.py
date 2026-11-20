import os

from pyinfra import host
from pyinfra.operations import files
from lib.operations import kubernetes

target_user = host.data.get('target_user')
kubeconfig_path = f"/home/{target_user}/.kube/config"

calico_operator_crds_manifest = host.data.get('calico_operator_crds_manifest')
calico_operator_manifest = host.data.get('calico_operator_manifest')
calico_custom_resources_manifest = host.data.get('calico_custom_resources_manifest')

# Apply Calico operator CRDs
kubernetes.apply(
    name="Apply Calico Operator CRDs",
    manifest_path=calico_operator_crds_manifest,
    server_side=True,
    kubeconfig=kubeconfig_path
)

# Apply Calico operator manifest and wait for it to be ready
kubernetes.apply(
    name="Apply Calico operator manifest",
    manifest_path=calico_operator_manifest,
    kubeconfig=kubeconfig_path,
    wait_resource="deployment/tigera-operator",
    wait_condition="Available",
    wait_namespace="tigera-operator",
    wait_timeout="300s"
)

# Render Calico custom resources so the IP pool matches kubeadm's --pod-network-cidr
files.directory(
    name="Create Kubernetes addons directory",
    path=os.path.dirname(calico_custom_resources_manifest),
    mode="0755",
    present=True,
    _sudo=True
)

files.template(
    name="Render Calico custom resources manifest",
    src="templates/calico-custom-resources-bpf.yaml.j2",
    dest=calico_custom_resources_manifest,
    mode="0644",
    calico_version=host.data.get('calico_version'),
    pod_network_cidr=host.data.get('pod_network_cidr'),
    _sudo=True
)

# Apply Calico custom resources manifest
kubernetes.apply(
    name="Apply Calico custom resources manifest",
    manifest_path=calico_custom_resources_manifest,
    kubeconfig=kubeconfig_path
)

# Remove control-plane taint
kubernetes.taint_nodes(
    name="Remove control-plane taint",
    taint_string="node-role.kubernetes.io/control-plane-",
    kubeconfig=kubeconfig_path
)

# Wait for node to be ready after CNI initialization
kubernetes.wait_for_condition(
    name="Wait for Kubernetes node to be Ready after Calico init",
    resource="nodes --all",
    condition="Ready",
    timeout="600s",
    kubeconfig=kubeconfig_path
)

# Point CoreDNS at public resolvers reachable from this network (unreachable
# ones are dropped, so a blocked resolver can't cause slow/failed lookups)
kubernetes.configure_coredns_upstreams(
    name="Configure CoreDNS upstream DNS servers",
    servers=host.data.get('cluster_dns_servers'),
    kubeconfig=kubeconfig_path
)

