#!/bin/bash

# Gateway API 测试脚本
# 用于测试 test.example.com 的 Gateway 配置

set -e

echo "=========================================="
echo "Gateway API 测试"
echo "域名: test.example.com"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 1. 部署测试配置
echo -e "${YELLOW}步骤 1: 部署 Gateway 和测试应用...${NC}"
kubectl apply -f examples/gateway-test.yaml

echo ""
echo -e "${YELLOW}步骤 2: 等待 Gateway 就绪...${NC}"
echo "等待 Gateway default-gateway 就绪（最多等待 60 秒）..."

for i in {1..60}; do
    STATUS=$(kubectl get gateway default-gateway -n default -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}' 2>/dev/null || echo "")
    if [ "$STATUS" = "True" ]; then
        echo -e "${GREEN}✓ Gateway 已就绪${NC}"
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}✗ Gateway 未能在 60 秒内就绪${NC}"
        kubectl get gateway default-gateway -n default
        exit 1
    fi
    echo -n "."
    sleep 1
done

echo ""
echo -e "${YELLOW}步骤 3: 等待 Echo Server Pod 就绪...${NC}"
kubectl wait --for=condition=ready pod -l app=echo-server -n default --timeout=60s

echo ""
echo -e "${YELLOW}步骤 4: 获取 Gateway 信息...${NC}"
GATEWAY_IP=$(kubectl get gateway default-gateway -n default -o jsonpath='{.status.addresses[0].value}')
echo -e "${GREEN}Gateway IP: $GATEWAY_IP${NC}"

echo ""
echo -e "${YELLOW}步骤 5: 检查 HTTPRoute 状态...${NC}"
kubectl get httproute test-route -n default

echo ""
echo "=========================================="
echo -e "${GREEN}测试 HTTP 访问${NC}"
echo "=========================================="

# 测试 1: 使用 Host 头访问
echo ""
echo "测试 1: 使用 Host 头访问"
echo "命令: curl -v -H 'Host: test.example.com' http://$GATEWAY_IP"
echo ""
curl -v -H "Host: test.example.com" http://$GATEWAY_IP

echo ""
echo ""
echo "=========================================="
echo -e "${GREEN}测试完成！${NC}"
echo "=========================================="
echo ""
echo "后续测试建议："
echo ""
echo "1. 配置 DNS 解析（或修改 /etc/hosts）："
echo "   echo '$GATEWAY_IP test.example.com' | sudo tee -a /etc/hosts"
echo ""
echo "2. 然后可以直接访问："
echo "   curl http://test.example.com"
echo ""
echo "3. 在浏览器访问："
echo "   http://test.example.com"
echo ""
echo "4. 查看实时流量（Hubble）："
echo "   hubble observe --namespace default --follow"
echo ""
echo "5. 查看 Gateway 详细信息："
echo "   kubectl describe gateway default-gateway -n default"
echo ""
echo "6. 清理测试资源："
echo "   kubectl delete -f examples/gateway-test.yaml"
echo ""

