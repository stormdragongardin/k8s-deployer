#!/bin/bash
# Kubernetes 镜像同步脚本 - 修复路径问题
# 使用 skopeo 将镜像从 registry.k8s.io 同步到 Harbor

set -e

HARBOR_HOST="${HARBOR_HOST:-harbor.example.com}"
HARBOR_PROJECT="${HARBOR_PROJECT:-k8s}"
K8S_VERSION="v1.34.2"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
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

# 检查 skopeo 是否安装
if ! command -v skopeo &> /dev/null; then
    log_error "skopeo 未安装，请先安装："
    echo "  Ubuntu/Debian: sudo apt-get install skopeo"
    echo "  CentOS/RHEL: sudo yum install skopeo"
    exit 1
fi

# Kubernetes 核心镜像列表（注意：使用简化的目标路径）
declare -A IMAGES=(
    # 源镜像 -> 目标镜像名（不含 tag）
    ["registry.k8s.io/kube-apiserver:${K8S_VERSION}"]="kube-apiserver"
    ["registry.k8s.io/kube-controller-manager:${K8S_VERSION}"]="kube-controller-manager"
    ["registry.k8s.io/kube-scheduler:${K8S_VERSION}"]="kube-scheduler"
    ["registry.k8s.io/kube-proxy:${K8S_VERSION}"]="kube-proxy"
    ["registry.k8s.io/coredns/coredns:v1.12.1"]="coredns"
    ["registry.k8s.io/pause:3.10.1"]="pause"
    ["registry.k8s.io/etcd:3.5.17-0"]="etcd"
    
    # Cilium 镜像（v1.14.5）
    ["quay.io/cilium/cilium:v1.14.5"]="cilium"
    ["quay.io/cilium/operator-generic:v1.14.5"]="operator-generic"
    ["quay.io/cilium/hubble-relay:v1.14.5"]="hubble-relay"
    ["quay.io/cilium/hubble-ui:v0.12.1"]="hubble-ui"
    ["quay.io/cilium/hubble-ui-backend:v0.12.1"]="hubble-ui-backend"
)

log_info "开始同步 Kubernetes 镜像到 Harbor: ${HARBOR_HOST}/${HARBOR_PROJECT}"
log_info "Kubernetes 版本: ${K8S_VERSION}"
echo ""

# 同步镜像（带重试机制）
sync_image() {
    local src_image="$1"
    local dest_image="$2"
    local dest_name="$3"
    local tag="$4"
    local max_retries=3
    local retry=0
    
    while [ $retry -lt $max_retries ]; do
        if [ $retry -gt 0 ]; then
            log_warn "重试 ($retry/$max_retries): ${dest_name}:${tag}"
            sleep 2
        fi
        
        if skopeo copy \
            --dest-tls-verify=false \
            --retry-times=3 \
            "docker://${src_image}" \
            "docker://${dest_image}"; then
            log_info "✓ 同步成功: ${dest_name}:${tag}"
            return 0
        fi
        
        retry=$((retry + 1))
    done
    
    log_error "✗ 同步失败（已重试 $max_retries 次）: ${dest_name}:${tag}"
    return 1
}

# 同步镜像
for src_image in "${!IMAGES[@]}"; do
    dest_name="${IMAGES[$src_image]}"
    
    # 提取 tag
    tag="${src_image##*:}"
    
    # 构建目标镜像路径（扁平化，不保留多级路径）
    dest_image="${HARBOR_HOST}/${HARBOR_PROJECT}/${dest_name}:${tag}"
    
    log_info "同步: ${src_image}"
    log_info "  -> ${dest_image}"
    
    if ! sync_image "${src_image}" "${dest_image}" "${dest_name}" "${tag}"; then
        log_error "请检查网络连接或手动同步该镜像"
        exit 1
    fi
    echo ""
done

log_info "=========================================="
log_info "所有镜像同步完成！"
log_info "=========================================="
echo ""
log_info "验证命令："
echo "  skopeo inspect docker://${HARBOR_HOST}/${HARBOR_PROJECT}/coredns:v1.12.1"
echo "  skopeo inspect docker://${HARBOR_HOST}/${HARBOR_PROJECT}/pause:3.10.1"

