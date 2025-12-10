#!/bin/bash
# 更新 Cilium 镜像仓库配置
# 用于将现有 Cilium 部署的镜像仓库切换到 Harbor

set -e

# 默认配置
HARBOR_HOST="${1:-harbor.example.com}"
HARBOR_PROJECT="${2:-k8s}"
NAMESPACE="${3:-kube-system}"
CILIUM_VERSION="v1.14.5"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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
echo "    更新 Cilium 镜像仓库配置"
echo "=========================================="
echo ""

log_info "目标镜像仓库: ${HARBOR_HOST}/${HARBOR_PROJECT}"
log_info "命名空间: ${NAMESPACE}"
log_info "Cilium 版本: ${CILIUM_VERSION}"
echo ""

# 检查 kubectl
if ! command -v kubectl &> /dev/null; then
    log_error "kubectl 未安装或不在 PATH 中"
    exit 1
fi

# 检查 helm
if ! command -v helm &> /dev/null; then
    log_error "helm 未安装或不在 PATH 中"
    exit 1
fi

# 检查 Cilium 是否已安装
log_step "1/4 检查 Cilium 部署状态"
if ! helm list -n ${NAMESPACE} | grep -q cilium; then
    log_error "Cilium 未安装或不是通过 Helm 部署的"
    exit 1
fi
log_info "✓ Cilium 已通过 Helm 部署"
echo ""

# 显示当前镜像
log_step "2/4 当前镜像配置"
kubectl get ds cilium -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].image}' | while read img; do
    log_info "Cilium: ${img}"
done
kubectl get deploy cilium-operator -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null | while read img; do
    log_info "Operator: ${img}"
done
echo ""

# 更新配置
log_step "3/4 更新 Cilium 镜像配置"
log_info "使用 kubectl 直接更新 DaemonSet 和 Deployment..."

# 更新 Cilium DaemonSet
kubectl set image ds/cilium \
    -n ${NAMESPACE} \
    cilium-agent=${HARBOR_HOST}/${HARBOR_PROJECT}/cilium:${CILIUM_VERSION}

log_info "✓ Cilium DaemonSet 镜像已更新"

# 更新 Cilium Operator Deployment
kubectl set image deploy/cilium-operator \
    -n ${NAMESPACE} \
    cilium-operator=${HARBOR_HOST}/${HARBOR_PROJECT}/operator-generic:${CILIUM_VERSION}

log_info "✓ Cilium Operator 镜像已更新"

# 如果有 Hubble Relay，也更新它
if kubectl get deploy hubble-relay -n ${NAMESPACE} &>/dev/null; then
    kubectl set image deploy/hubble-relay \
        -n ${NAMESPACE} \
        hubble-relay=${HARBOR_HOST}/${HARBOR_PROJECT}/hubble-relay:${CILIUM_VERSION}
    log_info "✓ Hubble Relay 镜像已更新"
fi

echo ""

# 等待更新
log_step "4/4 等待 Cilium Pods 滚动重启"
log_info "等待 Cilium DaemonSet 更新..."

if kubectl rollout status ds/cilium -n ${NAMESPACE} --timeout=300s; then
    log_info "✓ Cilium DaemonSet 更新完成"
else
    log_warn "等待超时，请手动检查状态"
fi

if kubectl rollout status deploy/cilium-operator -n ${NAMESPACE} --timeout=300s 2>/dev/null; then
    log_info "✓ Cilium Operator 更新完成"
fi

echo ""
echo "=========================================="
echo "    更新完成！"
echo "=========================================="
echo ""

# 显示新镜像
log_info "新镜像配置："
kubectl get ds cilium -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].image}' | while read img; do
    echo "  Cilium: ${img}"
done
kubectl get deploy cilium-operator -n ${NAMESPACE} -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null | while read img; do
    echo "  Operator: ${img}"
done
echo ""

log_info "验证命令："
echo "  kubectl -n ${NAMESPACE} get pods -l k8s-app=cilium"
echo "  kubectl -n ${NAMESPACE} get pods -l io.cilium/app=operator"
echo ""

