#!/bin/bash
# 下载 GPU 相关的 deb 包
# 使用方法: ./download-gpu.sh

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="$PROJECT_ROOT/packages/gpu"

# 版本配置
NVIDIA_DRIVER_VERSION="580"
NVIDIA_TOOLKIT_VERSION="1.18.0"

echo "============================================"
echo "  下载 GPU 驱动和工具包"
echo "============================================"
echo ""
echo "NVIDIA Driver: $NVIDIA_DRIVER_VERSION-server-open"
echo "NVIDIA Container Toolkit: v$NVIDIA_TOOLKIT_VERSION"
echo ""

mkdir -p "$PACKAGE_DIR"

# 方法 1: 下载预打包的 nvidia-container-toolkit（推荐）
echo "方法 1: 下载 NVIDIA Container Toolkit 预打包版本"
echo ""

TOOLKIT_URL="https://github.com/NVIDIA/nvidia-container-toolkit/releases/download/v${NVIDIA_TOOLKIT_VERSION}/nvidia-container-toolkit_${NVIDIA_TOOLKIT_VERSION}_deb_amd64.tar.gz"
TOOLKIT_FILE="$PACKAGE_DIR/nvidia-container-toolkit_${NVIDIA_TOOLKIT_VERSION}_deb_amd64.tar.gz"

if [ -f "$TOOLKIT_FILE" ]; then
    echo "✓ nvidia-container-toolkit 已下载"
else
    echo "→ 下载 nvidia-container-toolkit..."
    wget -q --show-progress -O "$TOOLKIT_FILE" "$TOOLKIT_URL" || {
        echo "✗ 下载失败"
        exit 1
    }
    echo "✓ 下载完成"
fi

# 解压查看包含的文件
echo ""
echo "→ 解压 nvidia-container-toolkit..."
cd "$PACKAGE_DIR"
tar xzf "nvidia-container-toolkit_${NVIDIA_TOOLKIT_VERSION}_deb_amd64.tar.gz"
echo "✓ 解压完成"
echo ""

echo "包含的 deb 文件:"
ls -lh *.deb 2>/dev/null || echo "  (使用 tar 包安装)"
echo ""

# 方法 2: 下载 NVIDIA 驱动（需要在目标系统上下载）
echo "============================================"
echo "方法 2: 下载 NVIDIA 驱动"
echo "============================================"
echo ""
echo "NVIDIA 驱动需要在目标系统上下载，因为它依赖于："
echo "  - 内核版本"
echo "  - 系统架构"
echo "  - 发行版版本"
echo ""
echo "推荐方式:"
echo ""
echo "1. 在目标 Ubuntu 22.04 系统上运行:"
echo ""
echo "   # 添加 NVIDIA 驱动 PPA"
echo "   sudo add-apt-repository ppa:graphics-drivers/ppa"
echo "   sudo apt update"
echo ""
echo "   # 下载驱动包（不安装）"
echo "   cd $PACKAGE_DIR"
echo "   apt download nvidia-driver-${NVIDIA_DRIVER_VERSION}-server-open"
echo "   apt download nvidia-dkms-${NVIDIA_DRIVER_VERSION}-server-open"
echo "   apt download nvidia-kernel-common-${NVIDIA_DRIVER_VERSION}-server-open"
echo "   apt download nvidia-utils-${NVIDIA_DRIVER_VERSION}-server-open"
echo ""
echo "2. 或使用 Docker 容器下载（自动化）:"
echo ""

# 创建 Docker 自动下载脚本
cat > "$PACKAGE_DIR/download-driver-in-docker.sh" << 'DRIVER_SCRIPT'
#!/bin/bash
# 在 Docker 容器中下载 NVIDIA 驱动

DRIVER_VERSION="580"

docker run --rm -v "$(pwd):/output" ubuntu:22.04 bash -c "
    export DEBIAN_FRONTEND=noninteractive
    apt update
    apt install -y software-properties-common curl gnupg
    
    # 添加 NVIDIA PPA
    add-apt-repository -y ppa:graphics-drivers/ppa
    apt update
    
    # 下载 NVIDIA 驱动
    cd /output
    apt download nvidia-driver-${DRIVER_VERSION}-server-open 2>/dev/null || echo '⚠ nvidia-driver not found'
    apt download nvidia-dkms-${DRIVER_VERSION}-server-open 2>/dev/null || echo '⚠ nvidia-dkms not found'
    apt download nvidia-kernel-common-${DRIVER_VERSION}-server-open 2>/dev/null || echo '⚠ nvidia-kernel-common not found'
    apt download nvidia-kernel-source-${DRIVER_VERSION}-server-open 2>/dev/null || echo '⚠ nvidia-kernel-source not found'
    apt download nvidia-utils-${DRIVER_VERSION}-server-open 2>/dev/null || echo '⚠ nvidia-utils not found'
    
    echo ''
    echo '✓ 驱动下载完成'
    echo ''
    echo '下载的文件:'
    ls -lh /output/*.deb 2>/dev/null | grep nvidia-driver || echo '(无驱动包)'
"
DRIVER_SCRIPT

chmod +x "$PACKAGE_DIR/download-driver-in-docker.sh"

echo "   cd $PACKAGE_DIR"
echo "   ./download-driver-in-docker.sh"
echo ""

# 总结
echo "============================================"
echo "  ✓ nvidia-container-toolkit 下载完成"
echo "============================================"
echo ""
echo "下载的文件:"
ls -lh "$PACKAGE_DIR"/*.tar.gz 2>/dev/null
ls -lh "$PACKAGE_DIR"/*.deb 2>/dev/null | head -10
echo ""
echo "下一步:"
echo "  1. 下载 NVIDIA 驱动（如需要）:"
echo "     cd $PACKAGE_DIR && ./download-driver-in-docker.sh"
echo ""
echo "  2. 验证所有包:"
echo "     ../scripts/verify-packages.sh"
echo ""
echo "  3. 部署集群时会自动使用这些包"
echo ""


