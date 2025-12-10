#!/bin/bash
# 验证所有包是否完整

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="$PROJECT_ROOT/packages"

echo "============================================"
echo "  验证软件包"
echo "============================================"
echo ""

check_file() {
    local file=$1
    if [ -f "$file" ]; then
        local size=$(du -h "$file" | cut -f1)
        echo "  ✓ $file ($size)"
        return 0
    else
        echo "  ✗ 缺失: $file"
        return 1
    fi
}

failed=0

# 检查 containerd
echo "containerd:"
check_file "$PACKAGE_DIR/containerd/containerd-"*"-linux-amd64.tar.gz" || failed=$((failed+1))
check_file "$PACKAGE_DIR/containerd/runc.amd64" || failed=$((failed+1))
check_file "$PACKAGE_DIR/containerd/cni-plugins-linux-amd64-"*".tgz" || failed=$((failed+1))
echo ""

# 检查 Kubernetes
echo "Kubernetes:"
for k8s_dir in "$PACKAGE_DIR/kubernetes"/*/; do
    version=$(basename "$k8s_dir")
    echo "  版本: $version"
    check_file "$k8s_dir/kubectl" || failed=$((failed+1))
    check_file "$k8s_dir/kubeadm" || failed=$((failed+1))
    check_file "$k8s_dir/kubelet" || failed=$((failed+1))
done
echo ""

# 检查 Helm
echo "Helm:"
check_file "$PACKAGE_DIR/helm/helm-"*"-linux-amd64.tar.gz" || failed=$((failed+1))
echo ""

# 检查 Cilium
echo "Cilium:"
if [ -f "$PACKAGE_DIR/cilium/cilium-linux-amd64.tar.gz" ]; then
    check_file "$PACKAGE_DIR/cilium/cilium-linux-amd64.tar.gz"
else
    echo "  ⚠ Cilium CLI 未下载 (可选)"
fi
echo ""

# 检查 GPU 包
echo "GPU 包:"
gpu_count=$(find "$PACKAGE_DIR/gpu" -name "*.deb" 2>/dev/null | wc -l)
if [ $gpu_count -gt 0 ]; then
    echo "  ✓ 找到 $gpu_count 个 GPU 相关包"
    find "$PACKAGE_DIR/gpu" -name "*.deb" -exec basename {} \; | sed 's/^/    - /'
else
    echo "  ⚠ GPU 包未下载 (如需 GPU 支持，请运行 ./scripts/download-gpu.sh)"
fi
echo ""

# 总结
echo "============================================"
if [ $failed -eq 0 ]; then
    echo "  ✓ 所有必需包验证通过"
else
    echo "  ✗ 有 $failed 个必需包缺失"
    echo ""
    echo "请运行以下命令下载缺失的包:"
    echo "  ./scripts/download-all.sh"
fi
echo "============================================"
echo ""

# 显示总大小
total_size=$(du -sh "$PACKAGE_DIR" 2>/dev/null | cut -f1)
echo "包总大小: $total_size"
echo ""

exit $failed

