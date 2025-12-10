#!/bin/bash
# 批量清理集群所有节点
# 读取集群配置文件，并在所有节点上执行清理

set -e

CLUSTER_CONFIG="${1:-aigc-cluster.yaml}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLEANUP_SCRIPT="${SCRIPT_DIR}/cleanup-cluster.sh"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

echo "=========================================="
echo "  批量清理所有节点"
echo "=========================================="
echo ""

# 检查配置文件
if [ ! -f "$CLUSTER_CONFIG" ]; then
    echo "错误: 找不到配置文件 $CLUSTER_CONFIG"
    exit 1
fi

# 检查清理脚本
if [ ! -f "$CLEANUP_SCRIPT" ]; then
    echo "错误: 找不到清理脚本 $CLEANUP_SCRIPT"
    exit 1
fi

log_info "配置文件: $CLUSTER_CONFIG"
log_info "清理脚本: $CLEANUP_SCRIPT"
echo ""

# 解析节点 IP（简单 grep，支持 YAML 格式）
NODES=$(grep -E "^\s+ip:\s+" "$CLUSTER_CONFIG" | awk '{print $2}' | sort -u)

if [ -z "$NODES" ]; then
    echo "错误: 未找到任何节点 IP"
    exit 1
fi

echo "发现以下节点："
echo "$NODES" | while read ip; do
    echo "  • $ip"
done
echo ""

log_warn "这将清理所有节点的 Kubernetes 配置！"
read -p "确认继续？(yes/no): " -r
if [[ ! $REPLY =~ ^yes$ ]]; then
    log_info "已取消"
    exit 0
fi
echo ""

# 执行清理
NODE_NUM=1
TOTAL=$(echo "$NODES" | wc -l)

echo "$NODES" | while read ip; do
    log_step "[$NODE_NUM/$TOTAL] 清理节点: $ip"
    
    # 复制清理脚本到节点
    if scp -o StrictHostKeyChecking=no "$CLEANUP_SCRIPT" root@$ip:/tmp/cleanup-cluster.sh; then
        # 执行清理
        if ssh -o StrictHostKeyChecking=no root@$ip "chmod +x /tmp/cleanup-cluster.sh && echo 'yes' | /tmp/cleanup-cluster.sh"; then
            log_info "  ✓ 节点 $ip 清理完成"
        else
            log_warn "  ✗ 节点 $ip 清理失败"
        fi
        
        # 清理临时文件
        ssh -o StrictHostKeyChecking=no root@$ip "rm -f /tmp/cleanup-cluster.sh" 2>/dev/null || true
    else
        log_warn "  ✗ 无法连接到节点 $ip"
    fi
    
    echo ""
    NODE_NUM=$((NODE_NUM + 1))
done

echo "=========================================="
echo "  所有节点清理完成！"
echo "=========================================="
echo ""
log_info "下一步："
echo "  1. 验证所有节点状态正常"
echo "  2. 运行: ./k8s-deployer cluster create -f $CLUSTER_CONFIG --skip-ssh-setup"
echo ""

