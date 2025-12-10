# K8s Deployer

Kubernetes 集群离线部署工具，专为 AIGC/GPU 计算场景设计。

## 特性

- 🚀 **高可用集群** - 支持多 Master + Worker 节点
- 🎯 **GPU 节点** - 自动安装 NVIDIA 驱动和容器运行时
- 🌐 **Cilium 网络** - 替代 kube-proxy，eBPF 高性能
- 📦 **完全离线** - 所有组件预下载，支持内网部署
- 🔑 **自动化 SSH** - 自动配置密钥免密登录
- ⚡ **系统优化** - 自动配置内核参数和资源限制

## 组件版本

| 组件 | 版本 |
|------|------|
| Kubernetes | v1.34.2 |
| containerd | 2.2.0 |
| Cilium | 1.18.4 |
| Helm | v4.0.0 |
| NVIDIA Driver | 580-server-open |

## 快速开始

### 1. 下载离线包（必需）

> ⚠️ **重要**: 仓库不包含二进制文件，需先运行脚本下载！

```bash
# 克隆仓库
git clone https://github.com/your-org/k8s-deployer.git
cd k8s-deployer

# 下载所有依赖（约 500MB）
./scripts/download-all.sh --k8s-version v1.34.2

# GPU 节点需要额外下载
./scripts/download-gpu.sh

# 验证包完整性
./scripts/verify-packages.sh
```

### 2. 编译安装

```bash
make build
sudo cp bin/k8s-deployer /usr/local/bin/
```

### 3. 配置集群

创建配置文件 `my-cluster.yaml`：

```yaml
apiVersion: k8s-deployer/v1
kind: Cluster
metadata:
  name: my-cluster

spec:
  version: v1.34.2
  imageRepository: harbor.example.com/k8s
  
  networking:
    podSubnet: 10.244.0.0/16
    serviceSubnet: 10.96.0.0/12
  
  # Hubble 网络可观测性（可选）
  hubble:
    enabled: true
    ui:
      enabled: true
      nodePort: 31234
  
  nodes:
    # Master 节点
    - role: master
      ip: 192.168.1.11
      hostname: master-01
      ssh:
        user: admin
        password: "your-password"
        port: 22
    
    # 普通 Worker 节点
    - role: worker
      ip: 192.168.1.21
      hostname: node-01
      ssh:
        user: admin
        password: "your-password"
        port: 22
    
    # GPU Worker 节点
    - role: worker
      ip: 192.168.1.31
      hostname: gpu-node-01
      gpu: true
      ssh:
        user: admin
        password: "your-password"
        port: 22
```

### 4. 部署集群

```bash
# 一键部署
k8s-deployer cluster create -f my-cluster.yaml

# 自动确认所有提示
k8s-deployer cluster create -f my-cluster.yaml -y

# 跳过 SSH 密钥配置（已配置过）
k8s-deployer cluster create -f my-cluster.yaml --skip-ssh-setup
```

### 5. 验证集群

```bash
kubectl get nodes
kubectl get pods -n kube-system
```

## 命令参考

### 集群部署

```bash
# 创建集群
k8s-deployer cluster create -f config.yaml

# 更新集群配置
k8s-deployer cluster update -f config.yaml
```

### SSH 密钥

```bash
# 单独配置 SSH 密钥
k8s-deployer init ssh-key -f config.yaml
```

### 二进制管理

```bash
k8s-deployer binary download    # 下载
k8s-deployer binary list        # 列表
k8s-deployer binary clean       # 清理
```

## 部署流程

```
1. 检查 SSH 连接
2. 配置 SSH 密钥免密登录
3. 配置 /etc/hosts（节点互通）
4. 系统优化（sysctl、内核模块）
5. 安装 containerd + K8s 组件
6. 初始化 Master（kubeadm init）
7. 安装 Cilium 网络
8. 加入 Worker 节点
9. 配置 GPU 节点
10. 验证集群
```

## GPU 节点

配置 `gpu: true` 后自动执行：

- 安装 NVIDIA 驱动 (`nvidia-driver-580-server-open`)
- 锁定驱动版本
- 安装 nvidia-container-toolkit
- 配置 containerd nvidia runtime
- 打标签 `gpu=on`

调度 Pod 到 GPU 节点：

```yaml
nodeSelector:
  gpu: "on"
```

## 高可用模式

多 Master 节点 + VIP：

```yaml
spec:
  ha:
    enabled: true
    vip: 192.168.1.100
  
  nodes:
    - role: master
      ip: 192.168.1.11
    - role: master
      ip: 192.168.1.12
    - role: master
      ip: 192.168.1.13
```

## Hubble 可观测性

启用后通过 NodePort 访问 UI：

```yaml
hubble:
  enabled: true
  metrics:
    enabled: true
  ui:
    enabled: true
    nodePort: 31234
```

访问: `http://<节点IP>:31234`

## 目录结构

```
k8s-deployer/
├── cmd/k8s-deployer/     # 入口
├── pkg/                  # 核心代码
├── packages/             # 离线安装包
│   ├── cilium/
│   ├── containerd/
│   ├── gpu/
│   ├── helm/
│   └── kubernetes/
├── configs/              # 配置示例
└── scripts/              # 辅助脚本
```

## 系统要求

**部署机：**
- Go 1.25+
- SSH 访问所有节点

**目标节点：**
- Ubuntu 22.04 / 24.04
- 2+ CPU、4GB+ 内存
- root 或 sudo 权限

## 进阶

- [BGP 模式部署](README-BGP.md) - 使用 BGP 实现 LoadBalancer 和 Gateway API

## 开发

```bash
make deps    # 依赖
make build   # 编译
make test    # 测试
make clean   # 清理
```

## 许可证

MIT
