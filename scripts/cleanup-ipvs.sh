#!/bin/bash
# 清理 IPVS 配置（Cilium eBPF 不需要）
# 在所有节点上运行

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=========================================="
echo "  清理 IPVS 模块配置（Cilium eBPF 模式）"
echo "=========================================="
echo ""

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# 1. 卸载 IPVS 内核模块
log_info "1. 卸载 IPVS 内核模块..."
for mod in ip_vs_sh ip_vs_wrr ip_vs_rr ip_vs; do
    if lsmod | grep -q "^${mod}"; then
        log_info "  卸载模块: ${mod}"
        rmmod ${mod} 2>/dev/null || log_warn "  无法卸载 ${mod}，可能被占用"
    fi
done

# 2. 更新模块自动加载配置
log_info "2. 更新 /etc/modules-load.d/k8s.conf..."
cat > /etc/modules-load.d/k8s.conf << 'EOF'
# Kubernetes 必需的内核模块（针对 Cilium eBPF 优化）
# 文件路径: /etc/modules-load.d/k8s.conf

# Overlay 文件系统（容器存储）
overlay

# 桥接网络（保留用于 CNI）
br_netfilter

# 连接跟踪（Cilium 需要）
nf_conntrack

# 注意：使用 Cilium eBPF kube-proxy replacement 模式时
# 不需要 IPVS 模块（ip_vs, ip_vs_rr, ip_vs_wrr, ip_vs_sh）
# Cilium 直接通过 eBPF 处理负载均衡，性能更优
EOF

log_info "  ✓ 配置已更新"

# 3. 清理 IPVS 规则（如果有）
log_info "3. 清理 IPVS 规则..."
if command -v ipvsadm &> /dev/null; then
    ipvsadm -C 2>/dev/null || true
    log_info "  ✓ IPVS 规则已清理"
else
    log_info "  ipvsadm 未安装，跳过"
fi

# 4. 可选：卸载 ipvsadm 工具
read -p "是否卸载 ipvsadm 工具？(y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    log_info "4. 卸载 ipvsadm..."
    apt-get remove -y ipvsadm 2>/dev/null || yum remove -y ipvsadm 2>/dev/null || true
    log_info "  ✓ ipvsadm 已卸载"
else
    log_info "4. 保留 ipvsadm 工具"
fi

echo ""
echo "=========================================="
echo "  清理完成！"
echo "=========================================="
echo ""
log_info "已完成的操作："
echo "  ✓ 卸载 IPVS 内核模块"
echo "  ✓ 更新模块自动加载配置"
echo "  ✓ 清理 IPVS 规则"
echo ""
log_info "验证："
echo "  lsmod | grep ip_vs  # 应该没有输出"
echo "  cat /etc/modules-load.d/k8s.conf"
echo ""
log_warn "注意：重启后这些设置才会完全生效"

