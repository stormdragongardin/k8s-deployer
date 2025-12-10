# K8s Deployer - BGP 模式部署

本文档介绍如何使用 Cilium BGP Control Plane 部署 Kubernetes 集群，实现：

- **LoadBalancer Service** - 无需云厂商，本地也能用 LoadBalancer
- **Gateway API** - 现代化的 Ingress 替代方案
- **DSR 模式** - Direct Server Return，高性能负载均衡

## 架构概览

```
                    ┌─────────────────┐
                    │   交换机/路由器  │
                    │   AS 65000      │
                    │   10.0.4.250    │
                    └────────┬────────┘
                             │ BGP Peering
            ┌────────────────┼────────────────┐
            │                │                │
     ┌──────┴──────┐  ┌──────┴──────┐  ┌──────┴──────┐
     │   Master    │  │   Worker    │  │   Worker    │
     │  AS 65001   │  │  AS 65001   │  │  AS 65001   │
     │ 10.0.4.109  │  │ 10.0.4.110  │  │ 10.0.4.111  │
     └─────────────┘  └─────────────┘  └─────────────┘
                             │
                    LoadBalancer VIPs
                      10.0.6.0/24
```

## 网络规划

> ⚠️ **重要**: LoadBalancer IP 必须使用独立网段！

| 网络 | 网段 | 用途 |
|------|------|------|
| 节点网络 | 10.0.4.0/24 | 物理节点 IP |
| Pod 网络 | 10.60.0.0/16 | Pod CIDR |
| Service 网络 | 10.70.0.0/16 | ClusterIP |
| **LB 网络** | **10.0.6.0/24** | LoadBalancer VIPs |

**为什么需要独立网段？**

如果 LB IP 与节点在同一子网（如都在 10.0.4.0/24），交换机会因为 "Supernet Route" 问题无法正确路由 BGP 通告的路由。

## 交换机配置

在交换机上配置 BGP 对等体：

### 华为交换机示例

```
# 创建 BGP 进程
bgp 65000
 router-id 10.0.4.250
 
 # 添加所有 K8s 节点为 peer
 peer 10.0.4.109 as-number 65001
 peer 10.0.4.110 as-number 65001
 peer 10.0.4.111 as-number 65001
 
 # IPv4 地址族
 ipv4-family unicast
  peer 10.0.4.109 enable
  peer 10.0.4.110 enable
  peer 10.0.4.111 enable

# 重要：添加 LB 网段的空路由（解决 Supernet 问题）
ip route-static 10.0.6.0 24 NULL 0
```

### 验证 BGP 状态

```
display bgp peer
display ip routing-table protocol bgp
```

## 集群配置

完整的 BGP 模式配置：

```yaml
apiVersion: k8s-deployer/v1
kind: Cluster
metadata:
  name: aigc

spec:
  version: v1.34.2
  imageRepository: harbor.example.com/k8s
  
  networking:
    podSubnet: 10.60.0.0/16
    serviceSubnet: 10.70.0.0/16
  
  # ========================================
  # Hubble 可观测性
  # ========================================
  hubble:
    enabled: true
    metrics:
      enabled: true
    ui:
      enabled: true
      nodePort: 31234
  
  # ========================================
  # LoadBalancer 配置
  # ========================================
  loadBalancer:
    provider: cilium   # 使用 Cilium BGP
    mode: dsr          # DSR 高性能模式
  
  # ========================================
  # BGP 配置
  # ========================================
  bgp:
    enabled: true
    localASN: 65001                    # 集群 AS 号
    
    peers:
      - peerAddress: 10.0.4.250        # 交换机 IP
        peerASN: 65000                 # 交换机 AS 号
    
    # LoadBalancer IP 池（独立网段）
    loadBalancerIPs:
      - 10.0.6.1-10.0.6.254            # 共 254 个 IP
  
  # ========================================
  # Gateway API（L7 路由）
  # ========================================
  gatewayAPI:
    enabled: true
  
  # ========================================
  # Envoy（Gateway API 需要）
  # ========================================
  envoy:
    enabled: true
  
  # ========================================
  # 节点配置
  # ========================================
  nodes:
    - role: master
      ip: 10.0.4.109
      hostname: master-01
      ssh:
        user: admin
        password: "your-password"
        port: 22
    
    - role: worker
      ip: 10.0.4.110
      hostname: node-01
      ssh:
        user: admin
        password: "your-password"
        port: 22
    
    - role: worker
      ip: 10.0.4.111
      hostname: gpu-node-01
      gpu: true
      ssh:
        user: admin
        password: "your-password"
        port: 22
```

## 部署

```bash
k8s-deployer cluster create -f aigc-cluster.yaml -y
```

## 验证 BGP

### 检查 Cilium BGP 状态

```bash
# 查看 BGP peering 状态
cilium bgp peers

# 查看通告的路由
cilium bgp routes advertised ipv4 unicast
```

### 检查交换机路由

在交换机上：

```
display ip routing-table protocol bgp
```

应该看到类似：

```
10.0.6.x/32  BGP  10.0.4.109  ...
10.0.6.x/32  BGP  10.0.4.110  ...
```

## 使用 LoadBalancer

创建 LoadBalancer Service：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  type: LoadBalancer
  ports:
    - port: 80
      targetPort: 8080
  selector:
    app: my-app
```

查看分配的 IP：

```bash
kubectl get svc my-service
# EXTERNAL-IP 会从 10.0.6.0/24 池中分配
```

## Gateway API

### 1. 创建 Gateway

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: default-gateway
  namespace: default
spec:
  gatewayClassName: cilium
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
  addresses:
    - type: IPAddress
      value: "10.0.6.1"    # 指定 LB IP
```

### 2. 创建 HTTPRoute

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
spec:
  parentRefs:
    - name: default-gateway
  hostnames:
    - "app.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /api
      backendRefs:
        - name: api-service
          port: 80
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: web-service
          port: 80
```

### 3. 验证

```bash
# 查看 Gateway 状态
kubectl get gateway

# 查看路由
kubectl get httproute

# 测试访问
curl -H "Host: app.example.com" http://10.0.6.1/api
```

## 更新 BGP 配置

修改配置后更新集群：

```bash
# 只更新 BGP 配置
k8s-deployer cluster update -f aigc-cluster.yaml --only-bgp

# 更新所有配置
k8s-deployer cluster update -f aigc-cluster.yaml
```

## 故障排查

### BGP 未建立

```bash
# 检查 Cilium agent 日志
kubectl -n kube-system logs -l k8s-app=cilium | grep -i bgp

# 检查节点是否有 BGP 配置
kubectl get ciliumbgppeeringpolicies -A
kubectl get ciliumloadbalancerippool -A
```

### LoadBalancer 无 EXTERNAL-IP

```bash
# 检查 IP 池
kubectl get ciliumloadbalancerippool -o yaml

# 检查是否有冲突
kubectl get svc -A | grep LoadBalancer
```

### Gateway 不工作

```bash
# 检查 GatewayClass
kubectl get gatewayclass

# 检查 Envoy 状态
kubectl -n kube-system get pods | grep envoy

# 检查 Gateway 状态
kubectl describe gateway default-gateway
```

## DSR 模式说明

DSR (Direct Server Return) 是高性能负载均衡模式：

```
Client → Switch → Node(LB) → Pod
                      ↓
Client ←────────────Pod (直接返回)
```

优点：
- 返回流量不经过 LB 节点
- 更低延迟
- 更高吞吐

注意：
- 需要二层网络支持
- 客户端和 Pod 需在同一网络

## 参考

- [Cilium BGP Control Plane](https://docs.cilium.io/en/stable/network/bgp-control-plane/)
- [Cilium LoadBalancer IPAM](https://docs.cilium.io/en/stable/network/lb-ipam/)
- [Gateway API](https://gateway-api.sigs.k8s.io/)

