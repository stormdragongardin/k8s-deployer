#!/bin/bash
# 下载所有部署所需的软件包
# 使用方法: ./download-all.sh --k8s-version v1.34.2

set -e

# 默认版本
K8S_VERSION="v1.34.2"
CONTAINERD_VERSION="2.2.0"
CNI_VERSION="v1.8.0"  # 最新稳定版
HELM_VERSION="v4.0.0"  # Helm 4.0 正式版 - 支持 WASM 插件、Server-side apply 等
RUNC_VERSION="v1.3.3"  # 修复高危安全漏洞 CVE-2025-31133, CVE-2025-52565, CVE-2025-52881

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --k8s-version)
            K8S_VERSION="$2"
            shift 2
            ;;
        --containerd-version)
            CONTAINERD_VERSION="$2"
            shift 2
            ;;
        *)
            echo "未知参数: $1"
            exit 1
            ;;
    esac
done

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="$PROJECT_ROOT/packages"

echo "============================================"
echo "  下载部署包"
echo "============================================"
echo ""
echo "Kubernetes 版本: $K8S_VERSION"
echo "containerd 版本: $CONTAINERD_VERSION"
echo "CNI 版本: $CNI_VERSION"
echo "Helm 版本: $HELM_VERSION"
echo ""

# 创建目录
mkdir -p "$PACKAGE_DIR"/{containerd,kubernetes/$K8S_VERSION,helm,cilium,gpu,system}

# 下载函数
download_file() {
    local url=$1
    local output=$2
    
    if [ -f "$output" ]; then
        echo "  ✓ 已存在: $(basename $output)"
        return 0
    fi
    
    echo "  → 下载: $(basename $output)"
    wget -q --show-progress -O "$output" "$url" || {
        echo "  ✗ 下载失败: $url"
        return 1
    }
    echo "  ✓ 完成"
}

# 1. 下载 containerd
echo "1. 下载 containerd..."
download_file \
    "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz" \
    "$PACKAGE_DIR/containerd/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz"

# 2. 下载 runc
echo "2. 下载 runc..."
download_file \
    "https://github.com/opencontainers/runc/releases/download/${RUNC_VERSION}/runc.amd64" \
    "$PACKAGE_DIR/containerd/runc.amd64"

# 3. 下载 CNI plugins
echo "3. 下载 CNI plugins..."
download_file \
    "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-amd64-${CNI_VERSION}.tgz" \
    "$PACKAGE_DIR/containerd/cni-plugins-linux-amd64-${CNI_VERSION}.tgz"

# 4. 下载 Kubernetes 组件
echo "4. 下载 Kubernetes 组件 ($K8S_VERSION)..."
K8S_BASE_URL="https://dl.k8s.io/release/${K8S_VERSION}/bin/linux/amd64"

for component in kubectl kubeadm kubelet; do
    download_file \
        "${K8S_BASE_URL}/${component}" \
        "$PACKAGE_DIR/kubernetes/${K8S_VERSION}/${component}"
    chmod +x "$PACKAGE_DIR/kubernetes/${K8S_VERSION}/${component}"
done

# 下载 kubelet.service
download_file \
    "https://raw.githubusercontent.com/kubernetes/release/master/cmd/krel/templates/latest/kubelet/kubelet.service" \
    "$PACKAGE_DIR/kubernetes/${K8S_VERSION}/kubelet.service"

# 5. 下载 Helm
echo "5. 下载 Helm..."
HELM_ARCHIVE="$PACKAGE_DIR/helm/helm-${HELM_VERSION}-linux-amd64.tar.gz"
download_file \
    "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" \
    "$HELM_ARCHIVE"

# 解压 Helm（如果还没解压）
if [ ! -f "$PACKAGE_DIR/helm/linux-amd64/helm" ]; then
    echo "  → 解压 Helm..."
    tar -xzf "$HELM_ARCHIVE" -C "$PACKAGE_DIR/helm/"
    echo "  ✓ Helm 解压完成"
fi

# 6. 下载 Cilium Helm Chart
echo "6. 下载 Cilium Helm Chart..."
CILIUM_CHART_VERSION="1.18.4"
download_file \
    "https://helm.cilium.io/cilium-${CILIUM_CHART_VERSION}.tgz" \
    "$PACKAGE_DIR/cilium/cilium-${CILIUM_CHART_VERSION}.tgz"

# 7. 下载 Cilium CLI (可选，用于调试)
echo "7. 下载 Cilium CLI..."
CILIUM_CLI_VERSION=$(curl -s https://raw.githubusercontent.com/cilium/cilium-cli/main/stable.txt)
download_file \
    "https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-amd64.tar.gz" \
    "$PACKAGE_DIR/cilium/cilium-linux-amd64.tar.gz"

echo ""
echo "============================================"
echo "  ✓ 所有包下载完成"
echo "============================================"
echo ""
echo "包存储位置: $PACKAGE_DIR"
echo ""
echo "下一步:"
echo "  1. 验证包: ./scripts/verify-packages.sh"
echo "  2. 下载 GPU 包 (可选): ./scripts/download-gpu.sh"
echo "  3. 打包传输: tar czf k8s-packages.tar.gz packages/"
echo ""

