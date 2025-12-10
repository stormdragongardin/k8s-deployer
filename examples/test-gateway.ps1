# Gateway API 测试脚本 (PowerShell)
# 用于测试 test.example.com 的 Gateway 配置

$ErrorActionPreference = "Stop"

Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "Gateway API 测试" -ForegroundColor Cyan
Write-Host "域名: test.example.com" -ForegroundColor Cyan
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host ""

# 1. 部署测试配置
Write-Host "步骤 1: 部署 Gateway 和测试应用..." -ForegroundColor Yellow
kubectl apply -f examples\gateway-test.yaml

Write-Host ""
Write-Host "步骤 2: 等待 Gateway 就绪..." -ForegroundColor Yellow
Write-Host "等待 Gateway default-gateway 就绪（最多等待 60 秒）..."

$ready = $false
for ($i = 1; $i -le 60; $i++) {
    try {
        $status = kubectl get gateway default-gateway -n default -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}' 2>$null
        if ($status -eq "True") {
            Write-Host "✓ Gateway 已就绪" -ForegroundColor Green
            $ready = $true
            break
        }
    } catch {
        # 忽略错误，继续等待
    }
    Write-Host "." -NoNewline
    Start-Sleep -Seconds 1
}

if (-not $ready) {
    Write-Host ""
    Write-Host "✗ Gateway 未能在 60 秒内就绪" -ForegroundColor Red
    kubectl get gateway default-gateway -n default
    exit 1
}

Write-Host ""
Write-Host "步骤 3: 等待 Echo Server Pod 就绪..." -ForegroundColor Yellow
kubectl wait --for=condition=ready pod -l app=echo-server -n default --timeout=60s

Write-Host ""
Write-Host "步骤 4: 获取 Gateway 信息..." -ForegroundColor Yellow
$gatewayIP = kubectl get gateway default-gateway -n default -o jsonpath='{.status.addresses[0].value}'
Write-Host "Gateway IP: $gatewayIP" -ForegroundColor Green

Write-Host ""
Write-Host "步骤 5: 检查 HTTPRoute 状态..." -ForegroundColor Yellow
kubectl get httproute test-route -n default

Write-Host ""
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "测试 HTTP 访问" -ForegroundColor Green
Write-Host "===========================================" -ForegroundColor Cyan

# 测试 1: 使用 Host 头访问
Write-Host ""
Write-Host "测试 1: 使用 Host 头访问"
Write-Host "命令: curl -H 'Host: test.example.com' http://$gatewayIP"
Write-Host ""

try {
    $response = Invoke-WebRequest -Uri "http://$gatewayIP" -Headers @{ Host = "test.example.com" } -UseBasicParsing
    Write-Host "HTTP 状态码: $($response.StatusCode)" -ForegroundColor Green
    Write-Host ""
    Write-Host "响应内容:" -ForegroundColor Yellow
    Write-Host $response.Content
} catch {
    Write-Host "请求失败: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host ""
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "测试完成！" -ForegroundColor Green
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "后续测试建议："
Write-Host ""
Write-Host "1. 配置 DNS 解析（或修改 hosts 文件）："
Write-Host "   Windows: C:\Windows\System32\drivers\etc\hosts"
Write-Host "   添加一行: $gatewayIP test.example.com" -ForegroundColor Yellow
Write-Host ""
Write-Host "2. 然后可以直接访问："
Write-Host "   curl http://test.example.com"
Write-Host "   或使用 PowerShell:"
Write-Host "   Invoke-WebRequest http://test.example.com" -ForegroundColor Yellow
Write-Host ""
Write-Host "3. 在浏览器访问："
Write-Host "   http://test.example.com" -ForegroundColor Yellow
Write-Host ""
Write-Host "4. 查看实时流量（Hubble）："
Write-Host "   hubble observe --namespace default --follow" -ForegroundColor Yellow
Write-Host ""
Write-Host "5. 查看 Gateway 详细信息："
Write-Host "   kubectl describe gateway default-gateway -n default" -ForegroundColor Yellow
Write-Host ""
Write-Host "6. 清理测试资源："
Write-Host "   kubectl delete -f examples\gateway-test.yaml" -ForegroundColor Yellow
Write-Host ""

