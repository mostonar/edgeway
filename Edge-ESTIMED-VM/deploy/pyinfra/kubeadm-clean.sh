#!/usr/bin/env bash
#
# kubeadm-clean.sh
#
# Completely cleans a kubeadm node so it can be initialized again.
#
# Usage:
#   sudo ./kubeadm-clean.sh
#

set -Eeuo pipefail

#############################################
# Colors
#############################################
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[ OK ]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }

#############################################
# Root check
#############################################
if [[ $EUID -ne 0 ]]; then
    fail "Run this script with sudo or as root."
    exit 1
fi

echo
warn "This will completely remove Kubernetes, kubeadm, CNI and Calico state."
sleep 3

#############################################
# Stop services
#############################################
info "Stopping services..."

systemctl stop kubelet 2>/dev/null || true
systemctl stop containerd 2>/dev/null || true
systemctl stop cri-o 2>/dev/null || true

#############################################
# kubeadm reset
#############################################
if command -v kubeadm >/dev/null 2>&1; then
    info "Running kubeadm reset..."
    kubeadm reset -f || true
fi

#############################################
# Remove containers and pods
#############################################
if command -v crictl >/dev/null 2>&1; then

    info "Removing Kubernetes containers..."

    PODS=$(crictl pods -q 2>/dev/null || true)

    if [[ -n "$PODS" ]]; then
        crictl stopp $PODS 2>/dev/null || true
        crictl rmp $PODS 2>/dev/null || true
    fi

    CONTAINERS=$(crictl ps -aq 2>/dev/null || true)

    if [[ -n "$CONTAINERS" ]]; then
        crictl rm $CONTAINERS 2>/dev/null || true
    fi
fi

#############################################
# Remove CNI namespaces
#############################################
info "Removing network namespaces..."

ip netns list 2>/dev/null | awk '{print $1}' | while read -r ns
do
    ip netns delete "$ns" 2>/dev/null || true
done

#############################################
# Remove Kubernetes directories
#############################################
info "Removing Kubernetes directories..."

DIRS=(
    /etc/kubernetes
    /var/lib/kubelet
    /var/lib/etcd
    /var/lib/cni
    /var/lib/calico
    /etc/cni/net.d
    "$HOME/.kube"
)

for d in "${DIRS[@]}"
do
    if [[ -e "$d" ]]; then
        rm -rf "$d"
    fi
done

#############################################
# Unmount Calico runtime (if mounted)
#############################################
if mountpoint -q /var/run/calico/cgroup 2>/dev/null; then
    info "Unmounting Calico cgroup..."
    umount -l /var/run/calico/cgroup || true
fi

rmdir /var/run/calico 2>/dev/null || true

#############################################
# Remove interfaces
#############################################
info "Removing Kubernetes interfaces..."

for iface in $(ip -o link show | awk -F': ' '{print $2}' | sed 's/@.*//')
do
    case "$iface" in
        cali*|cni*|flannel*|vxlan.calico|vxlan-v6.calico|bpfin.cali|bpfout.cali)
            ip link delete "$iface" 2>/dev/null || true
            ;;
    esac
done

#############################################
# Flush routes
#############################################
info "Removing CNI routes..."

ip route | awk '/proto bird|cali|vxlan|cni/ {print $1}' | while read -r route
do
    ip route del "$route" 2>/dev/null || true
done

#############################################
# Flush iptables
#############################################
info "Flushing iptables..."

for table in filter nat mangle raw security
do
    iptables -t "$table" -F 2>/dev/null || true
    iptables -t "$table" -X 2>/dev/null || true
done

ip6tables -F 2>/dev/null || true
ip6tables -X 2>/dev/null || true

#############################################
# Clear IPVS
#############################################
if command -v ipvsadm >/dev/null 2>&1; then
    info "Clearing IPVS..."
    ipvsadm --clear || true
fi

#############################################
# Restart runtime
#############################################
info "Restarting container runtime..."

systemctl restart containerd 2>/dev/null || true
systemctl restart cri-o 2>/dev/null || true

#############################################
# Verification
#############################################

echo
echo "================== VERIFICATION =================="

echo
echo "[Directories]"

for d in \
/etc/kubernetes \
/var/lib/kubelet \
/var/lib/etcd \
/var/lib/cni \
/var/lib/calico \
/etc/cni/net.d
do
    if [[ -e "$d" ]]; then
        warn "$d exists"
    else
        ok "$d removed"
    fi
done

echo
echo "[Interfaces]"

if ip link | grep -Eq 'cali|cni|flannel|vxlan|bpf'; then
    warn "Interfaces still present:"
    ip link | grep -E 'cali|cni|flannel|vxlan|bpf'
else
    ok "No Kubernetes interfaces."
fi

echo
echo "[Network Namespaces]"

if ip netns | grep -q .; then
    warn "Namespaces still present:"
    ip netns
else
    ok "No network namespaces."
fi

echo
echo "[CRI Pods]"

if command -v crictl >/dev/null 2>&1; then

    if crictl pods -q | grep -q .; then
        warn "Pod sandboxes remain:"
        crictl pods
    else
        ok "No pod sandboxes."
    fi

    echo

    if crictl ps -aq | grep -q .; then
        warn "Containers remain:"
        crictl ps -a
    else
        ok "No containers."
    fi
fi

echo
echo "[Services]"

if systemctl is-active --quiet containerd; then
    ok "containerd running."
else
    warn "containerd not running."
fi

if systemctl is-active --quiet kubelet; then
    warn "kubelet still running."
else
    ok "kubelet stopped."
fi

echo
echo "=================================================="

echo
ok "Cleanup completed."

echo
echo "Recommended:"
echo "  sudo reboot"
echo
echo "After reboot:"
echo "  sudo kubeadm init ..."
echo "  Install your CNI plugin."