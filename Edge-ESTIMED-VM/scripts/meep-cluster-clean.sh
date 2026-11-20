#!/usr/bin/env bash
#
# kubeadm-clean.sh
#
# Completely cleans a kubeadm node for re-initialization.
# WARNING: This deletes the Kubernetes cluster state on this node.
#
# Usage:
#   sudo bash kubeadm-clean.sh
#

set -Eeuo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[ OK ]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[FAIL]${NC} $*"; }

[[ $EUID -eq 0 ]] || {
    err "Run as root."
    exit 1
}

echo
warn "This will completely remove Kubernetes and CNI state."
sleep 3

###############################################################################
# MEEP Cleanup
###############################################################################

log "Resolving user for MEEP cleanup..."
if [ -n "${SUDO_USER:-}" ]; then
    REAL_USER="$SUDO_USER"
    REAL_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
else
    REAL_USER="root"
    REAL_HOME="/root"
fi

log "Running meepctl delete core and dep as $REAL_USER..."
sudo -u "$REAL_USER" bash -c "unset SUDO_UID SUDO_GID SUDO_USER; cd $REAL_HOME/etsi-mec-sandbox && ./bin/meepctl/meepctl delete core" || true
sudo -u "$REAL_USER" bash -c "unset SUDO_UID SUDO_GID SUDO_USER; cd $REAL_HOME/etsi-mec-sandbox && ./bin/meepctl/meepctl delete dep" || true

log "Deleting MEEP configurations and workspace..."
rm -rf "$REAL_HOME/etsi-mec-sandbox/bin/meepctl"
rm -rf "$REAL_HOME/.meep"
rm -rf "$REAL_HOME/gocode"

###############################################################################
# Delete Deployments
###############################################################################

log "Deleting all deployments in default and calico-system namespaces..."
timeout 10s kubectl --kubeconfig=/etc/kubernetes/admin.conf delete all --all -n default --force --grace-period=0 || true
timeout 10s kubectl --kubeconfig=/etc/kubernetes/admin.conf delete all --all -n calico-system --force --grace-period=0 || true

###############################################################################
# Flush Images
###############################################################################

log "Flushing Docker images..."
if command -v docker >/dev/null; then
    docker rmi -f $(docker images -aq) 2>/dev/null || true
fi

log "Flushing Containerd images..."
if command -v crictl >/dev/null; then
    crictl rmi --all 2>/dev/null || true
fi

###############################################################################
# kubeadm reset
###############################################################################

if command -v kubeadm >/dev/null; then
    log "Running kubeadm reset..."
    timeout 30s kubeadm reset -f || true
fi

###############################################################################
# Stop services and forcefully kill lingering port bindings
###############################################################################

log "Stopping services..."

systemctl stop kubelet 2>/dev/null || true
systemctl stop containerd 2>/dev/null || true
systemctl stop cri-o 2>/dev/null || true

log "Forcefully killing lingering Kubernetes processes and port bindings..."
pkill -9 -f "kube-apiserver|etcd|kube-controller-manager|kube-scheduler|kubelet|containerd-shim" || true
fuser -k -9 6443/tcp 2>/dev/null || true
fuser -k -9 10250/tcp 2>/dev/null || true
fuser -k -9 2379/tcp 2>/dev/null || true

###############################################################################
# Uninstall Docker and Containerd
###############################################################################

log "Uninstalling Docker and Containerd..."
apt-get purge -y --allow-change-held-packages docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin docker-ce-rootless-extras >/dev/null 2>&1 || true
apt-get autoremove -y --purge >/dev/null 2>&1 || true
rm -rf /var/lib/docker >/dev/null 2>&1 || true
rm -rf /var/lib/containerd >/dev/null 2>&1 || true

###############################################################################
# Uninstall Developer Tools (Golang, Node/NVM, Helm)
###############################################################################

log "Uninstalling Developer Tools (Golang, NVM, Helm)..."
rm -rf /usr/local/go >/dev/null 2>&1 || true
rm -rf "$REAL_HOME/.nvm" >/dev/null 2>&1 || true
if command -v snap >/dev/null; then
    snap remove helm >/dev/null 2>&1 || true
fi

###############################################################################
# Revert System and Profile Configurations
###############################################################################

log "Reverting system configurations..."
# Remove meep-docker-registry from hosts
sed -i '/meep-docker-registry/d' /etc/hosts || true

# Remove kernel modules load config
rm -f /etc/modules-load.d/k8s.conf >/dev/null 2>&1 || true

log "Reverting user profile (.bashrc)..."
if [ -f "$REAL_HOME/.bashrc" ]; then
    sed -i '/PYINFRA MANAGED - Go environment setup/d' "$REAL_HOME/.bashrc" || true
    sed -i '/export GOPATH/d' "$REAL_HOME/.bashrc" || true
    sed -i '/export PATH.*GOPATH/d' "$REAL_HOME/.bashrc" || true
    sed -i '/kubectl completion bash/d' "$REAL_HOME/.bashrc" || true
fi

###############################################################################
# Remove Kubernetes directories
###############################################################################

log "Force unmounting lingering kubelet mounts..."
for mount in $(mount | grep '/var/lib/kubelet' | awk '{print $3}'); do
    umount -f "$mount" 2>/dev/null || true
    umount -l "$mount" 2>/dev/null || true
done

log "Removing Kubernetes files..."

rm -rf \
    /etc/kubernetes \
    /var/lib/etcd \
    /var/lib/kubelet \
    /var/lib/cni \
    /var/lib/calico \
    /etc/cni/net.d \
    /var/run/calico \
    ~/.kube >/dev/null 2>&1 || true

###############################################################################
# Remove CNI interfaces
###############################################################################

log "Removing CNI interfaces..."

for iface in $(ip -o link | awk -F': ' '{print $2}' | sed 's/@.*//'); do
    case "$iface" in
        cali*|cni*|flannel*|vxlan.calico|vxlan-v6.calico|bpfin.cali|bpfout.cali)
            ip link delete "$iface" 2>/dev/null || true
            ;;
    esac
done

###############################################################################
# Remove network namespaces
###############################################################################

log "Removing CNI namespaces..."

ip netns | awk '{print $1}' | while read -r ns; do
    ip netns delete "$ns" 2>/dev/null || true
done

###############################################################################
# Flush iptables
###############################################################################

log "Flushing iptables..."

for table in filter nat mangle raw security; do
    iptables -t "$table" -F 2>/dev/null || true
    iptables -t "$table" -X 2>/dev/null || true
done

ip6tables -F 2>/dev/null || true
ip6tables -X 2>/dev/null || true

###############################################################################
# Remove IPVS
###############################################################################

if command -v ipvsadm >/dev/null; then
    log "Clearing IPVS..."
    ipvsadm --clear || true
fi

###############################################################################
# Cleanup containerd
###############################################################################

if command -v crictl >/dev/null; then
    log "Cleaning CRI..."

    PODS=$(crictl pods -q 2>/dev/null || true)
    if [ -n "$PODS" ]; then
        crictl stopp $PODS >/dev/null 2>&1 || true
        crictl rmp $PODS >/dev/null 2>&1 || true
    fi
    CONTAINERS=$(crictl ps -aq 2>/dev/null || true)
    if [ -n "$CONTAINERS" ]; then
        crictl rm $CONTAINERS >/dev/null 2>&1 || true
    fi
fi

###############################################################################
# Restart runtime
###############################################################################

log "Starting runtime..."

systemctl start containerd 2>/dev/null || true
systemctl start cri-o 2>/dev/null || true

###############################################################################
# Verification
###############################################################################

echo
echo "================ Verification ================"

echo
echo "Kubernetes directories:"
for d in \
    /etc/kubernetes \
    /var/lib/etcd \
    /var/lib/kubelet \
    /etc/cni/net.d \
    /var/lib/calico
do
    if [[ -e "$d" ]]; then
        warn "$d exists"
    else
        ok "$d removed"
    fi
done

echo
echo "Developer Tools & Configurations:"
for d in \
    "$REAL_HOME/.meep" \
    "$REAL_HOME/gocode" \
    "$REAL_HOME/.nvm" \
    /usr/local/go \
    /etc/modules-load.d/k8s.conf
do
    if [[ -e "$d" ]]; then
        warn "$d exists"
    else
        ok "$d removed"
    fi
done

if command -v helm >/dev/null; then
    warn "helm is still installed"
else
    ok "helm removed"
fi

if grep -q "meep-docker-registry" /etc/hosts; then
    warn "meep-docker-registry still in /etc/hosts"
else
    ok "/etc/hosts cleaned"
fi

echo
echo "Network interfaces:"
if ip link | grep -E 'cali|cni|flannel|vxlan|bpf' >/dev/null; then
    warn "Residual interfaces detected:"
    ip link | grep -E 'cali|cni|flannel|vxlan|bpf'
else
    ok "No Kubernetes interfaces found."
fi

echo
echo "Network namespaces:"
if ip netns | grep -q .; then
    warn "Residual namespaces:"
    ip netns
else
    ok "No CNI namespaces."
fi

echo
echo "CRI containers:"
if command -v crictl >/dev/null; then
    if crictl ps -a -q 2>/dev/null | grep -q .; then
        warn "Residual containers:"
        crictl ps -a 2>/dev/null
    else
        ok "No containers."
    fi

    echo

    if crictl pods -q 2>/dev/null | grep -q .; then
        warn "Residual pod sandboxes:"
        crictl pods 2>/dev/null
    else
        ok "No pod sandboxes."
    fi
fi

echo
echo "Kubelet:"
if systemctl is-active --quiet kubelet; then
    warn "kubelet is running."
else
    ok "kubelet stopped."
fi

echo
echo "Container runtime:"
if systemctl is-active --quiet containerd; then
    ok "containerd running."
elif systemctl is-active --quiet cri-o; then
    ok "CRI-O running."
else
    warn "No container runtime running."
fi

echo
echo "=============================================="

ok "Cleanup complete."

echo
echo "Recommended:"
echo "1. Reboot the node."
echo "2. Run kubeadm init."
echo "3. Install the CNI plugin."