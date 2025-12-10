#!/bin/bash
# 完全清理 Kubernetes 集群和所有配置
# 用于集群重置和重新部署

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

echo "=========================================="
echo "    完全清理 Kubernetes 集群"
echo "=========================================="
echo ""
log_warn "这将删除所有 Kubernetes 配置和数据！"
log_warn "包括：etcd 数据、证书、容器、网络配置等"
echo ""
read -p "确认继续？(yes/no): " -r
if [[ ! $REPLY =~ ^yes$ ]]; then
    log_info "已取消"
    exit 0
fi
echo ""

# ========================================
# Step 1: 重置 Kubernetes
# ========================================
log_step "1/8 重置 Kubernetes 集群"
if command -v kubeadm &> /dev/null; then
    kubeadm reset -f 2>/dev/null || true
    log_info "  ✓ kubeadm reset 完成"
else
    log_info "  kubeadm 未安装，跳过"
fi

# ========================================
# Step 2: 停止服务
# ========================================
log_step "2/8 停止相关服务"
systemctl stop kubelet 2>/dev/null || true
systemctl stop containerd 2>/dev/null || true
systemctl stop docker 2>/dev/null || true
log_info "  ✓ 服务已停止"

# ========================================
# Step 3: 清理容器和镜像
# ========================================
log_step "3/8 清理容器和镜像"
if command -v crictl &> /dev/null; then
    crictl rm $(crictl ps -aq) 2>/dev/null || true
    crictl rmi $(crictl images -q) 2>/dev/null || true
fi
log_info "  ✓ 容器清理完成"

# ========================================
# Step 4: 清理网络配置
# ========================================
log_step "4/8 清理网络配置"

# 清理 CNI 配置
rm -rf /etc/cni/net.d/* 2>/dev/null || true

# 清理 iptables 规则
iptables -F && iptables -t nat -F && iptables -t mangle -F && iptables -X 2>/dev/null || true
ip6tables -F && ip6tables -t nat -F && ip6tables -t mangle -F && ip6tables -X 2>/dev/null || true

# 清理 IPVS 规则
ipvsadm -C 2>/dev/null || true

# 清理虚拟网络接口
ip link del cilium_host 2>/dev/null || true
ip link del cilium_net 2>/dev/null || true
ip link del cilium_vxlan 2>/dev/null || true
ip link del cni0 2>/dev/null || true
ip link del flannel.1 2>/dev/null || true

# 清理路由
ip route flush proto bird 2>/dev/null || true

log_info "  ✓ 网络配置已清理"

# ========================================
# Step 5: 卸载 IPVS 模块
# ========================================
log_step "5/8 卸载 IPVS 内核模块"
for mod in ip_vs_sh ip_vs_wrr ip_vs_rr ip_vs; do
    rmmod ${mod} 2>/dev/null || true
done
log_info "  ✓ IPVS 模块已卸载"

# ========================================
# Step 6: 清理目录和文件
# ========================================
log_step "6/8 清理 Kubernetes 目录"

# Kubernetes 目录
rm -rf /etc/kubernetes/* 2>/dev/null || true
rm -rf /var/lib/kubelet/* 2>/dev/null || true
rm -rf /var/lib/etcd/* 2>/dev/null || true
rm -rf /var/lib/cni/* 2>/dev/null || true

# 容器运行时
rm -rf /var/lib/containerd/io.containerd.grpc.v1.cri/* 2>/dev/null || true
rm -rf /run/containerd/* 2>/dev/null || true

# kubectl 配置
rm -rf ~/.kube 2>/dev/null || true
rm -rf /root/.kube 2>/dev/null || true

# 临时文件
rm -rf /tmp/cilium* /tmp/helm* /tmp/*-values.yaml 2>/dev/null || true

log_info "  ✓ 目录清理完成"

# ========================================
# Step 7: 更新内核模块配置（移除 IPVS）
# ========================================
log_step "7/8 更新内核模块配置"
cat > /etc/modules-load.d/k8s.conf << 'EOF'
# Kubernetes 必需的内核模块（针对 Cilium eBPF 优化）
overlay
br_netfilter
nf_conntrack
EOF
log_info "  ✓ 内核模块配置已更新"

# ========================================
# Step 8: 重启服务
# ========================================
log_step "8/8 重启 containerd 服务"
systemctl daemon-reload
systemctl start containerd
systemctl enable containerd
sleep 2

if systemctl is-active --quiet containerd; then
    log_info "  ✓ containerd 已启动"
else
    log_error "  ✗ containerd 启动失败"
fi

echo ""
echo "=========================================="
echo "    清理完成！"
echo "=========================================="
echo ""
log_info "已清理的内容："
echo "  ✓ Kubernetes 集群配置"
echo "  ✓ etcd 数据"
echo "  ✓ 容器和镜像"
echo "  ✓ 网络配置（CNI、iptables、IPVS）"
echo "  ✓ IPVS 内核模块"
echo "  ✓ 所有相关目录"
echo ""
log_info "节点状态："
echo "  • containerd: $(systemctl is-active containerd)"
echo "  • kubelet: $(systemctl is-active kubelet 2>/dev/null || echo 'inactive')"
echo ""
log_info "现在可以重新部署集群了！"
echo ""

