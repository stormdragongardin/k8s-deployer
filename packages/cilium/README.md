# Cilium Chart 下载说明

本项目使用 Cilium v1.18.4 作为 CNI 网络插件。

## 下载 Cilium Chart

### 方法 1: 使用 Helm 命令（推荐）

```bash
# 添加 Cilium Helm 仓库
helm repo add cilium https://helm.cilium.io/
helm repo update

# 下载 v1.18.4 Chart
helm pull cilium/cilium --version 1.18.4 -d packages/cilium/

# 验证
ls packages/cilium/cilium-1.18.4.tgz
```

### 方法 2: 直接下载

```bash
wget https://helm.cilium.io/cilium-1.18.4.tgz -O packages/cilium/cilium-1.18.4.tgz
```

## 验证

```bash
helm show chart packages/cilium/cilium-1.18.4.tgz
```

## 离线镜像准备

如果使用私有镜像仓库，需要同步以下镜像：

```bash
# 设置你的 Harbor 地址
HARBOR="harbor.example.com/k8s"

# 核心镜像
docker pull quay.io/cilium/cilium:v1.18.4
docker tag quay.io/cilium/cilium:v1.18.4 $HARBOR/cilium:v1.18.4
docker push $HARBOR/cilium:v1.18.4

docker pull quay.io/cilium/operator-generic:v1.18.4
docker tag quay.io/cilium/operator-generic:v1.18.4 $HARBOR/operator-generic:v1.18.4
docker push $HARBOR/operator-generic:v1.18.4

# Hubble（可选）
docker pull quay.io/cilium/hubble-relay:v1.18.4
docker tag quay.io/cilium/hubble-relay:v1.18.4 $HARBOR/hubble-relay:v1.18.4
docker push $HARBOR/hubble-relay:v1.18.4
```

## 查看 Chart 默认镜像版本

```bash
helm show values packages/cilium/cilium-1.18.4.tgz | grep -A2 "image:"
```
