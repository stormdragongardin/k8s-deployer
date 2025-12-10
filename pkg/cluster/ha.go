package cluster

import (
	"fmt"
	"strings"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/logger"
	"stormdragon/k8s-deployer/pkg/ui"
	"go.uber.org/zap"
)

// SetupHA 配置高可用（Keepalived + HAProxy）
func SetupHA(cfg *config.ClusterConfig) error {
	ui.Header("配置高可用（Keepalived + HAProxy）")
	
	masterNodes := getMasterNodes(cfg)
	if len(masterNodes) < 3 {
		return fmt.Errorf("高可用模式至少需要 3 个 Master 节点")
	}
	
	// 在每个 Master 上安装 Keepalived 和 HAProxy
	for i, node := range masterNodes {
		priority := 100 - i*10 // 第一个节点优先级最高
		
		ui.Step(i+1, len(masterNodes), "配置 Master 节点: %s (优先级: %d)", node.Hostname, priority)
		
		if err := setupHAOnNode(cfg, &node, priority, i == 0); err != nil {
			return fmt.Errorf("配置节点 %s 失败: %w", node.Hostname, err)
		}
	}
	
	ui.Success("高可用配置完成！")
	ui.Info("VIP: %s", cfg.Spec.HA.VIP)
	ui.Info("所有 Master 节点已配置 Keepalived + HAProxy")
	
	return nil
}

// setupHAOnNode 在单个节点上配置 HA
func setupHAOnNode(cfg *config.ClusterConfig, node *config.NodeConfig, priority int, isMaster bool) error {
	client, err := executor.NewSSHClientWithPassword(
		node.IP,
		node.SSH.Port,
		node.SSH.User,
		node.SSH.KeyFile,
		node.SSH.Password,
	)
	if err != nil {
		return err
	}
	defer client.Close()
	
	// 1. 安装 Keepalived 和 HAProxy
	ui.SubStep("安装 Keepalived 和 HAProxy...")
	installScript := `
		export DEBIAN_FRONTEND=noninteractive
		apt-get update -qq
		apt-get install -y keepalived haproxy
	`
	if _, err := client.Execute(installScript); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("安装软件包失败: %w", err)
	}
	ui.SubStepDone()
	
	// 2. 配置 HAProxy
	ui.SubStep("配置 HAProxy...")
	if err := configureHAProxy(client, cfg); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	// 3. 配置 Keepalived
	ui.SubStep("配置 Keepalived...")
	state := "BACKUP"
	if isMaster {
		state = "MASTER"
	}
	if err := configureKeepalived(client, cfg, node, state, priority); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	// 4. 启动服务
	ui.SubStep("启动服务...")
	startScript := `
		systemctl enable haproxy
		systemctl restart haproxy
		systemctl enable keepalived
		systemctl restart keepalived
		
		sleep 2
		
		# 验证服务状态
		systemctl is-active haproxy || exit 1
		systemctl is-active keepalived || exit 1
	`
	if _, err := client.Execute(startScript); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("启动服务失败: %w", err)
	}
	ui.SubStepDone()
	
	logger.Info("节点 HA 配置完成",
		zap.String("node", node.Hostname),
		zap.String("ip", node.IP),
		zap.String("state", state),
		zap.Int("priority", priority))
	
	return nil
}

// configureHAProxy 配置 HAProxy
func configureHAProxy(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	// 生成后端服务器列表
	var backends strings.Builder
	for i, node := range cfg.Spec.Nodes {
		if node.Role == "master" {
			backends.WriteString(fmt.Sprintf("    server master-%d %s:6443 check inter 2000 rise 2 fall 3\n", i+1, node.IP))
		}
	}
	
	haproxyConfig := fmt.Sprintf(`global
    log /dev/log local0
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon
    maxconn 4000

defaults
    log     global
    mode    tcp
    option  tcplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000
    retries 3

# Kubernetes API Server Frontend
frontend k8s-api
    bind *:6443
    mode tcp
    option tcplog
    default_backend k8s-api-backend

# Kubernetes API Server Backend
backend k8s-api-backend
    mode tcp
    balance roundrobin
    option tcp-check
%s

# Stats page (可选)
listen stats
    bind *:8404
    mode http
    stats enable
    stats uri /
    stats refresh 10s
    stats auth admin:admin
`, backends.String())
	
	// 写入配置
	cmd := fmt.Sprintf("cat > /etc/haproxy/haproxy.cfg << 'EOF'\n%s\nEOF", haproxyConfig)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("写入 HAProxy 配置失败: %w", err)
	}
	
	// 验证配置
	if _, err := client.Execute("haproxy -c -f /etc/haproxy/haproxy.cfg"); err != nil {
		return fmt.Errorf("HAProxy 配置验证失败: %w", err)
	}
	
	return nil
}

// configureKeepalived 配置 Keepalived
func configureKeepalived(client *executor.SSHClient, cfg *config.ClusterConfig, node *config.NodeConfig, state string, priority int) error {
	// 检测网卡名称
	output, err := client.Execute("ip -o -4 route show to default | awk '{print $5}' | head -1")
	if err != nil {
		return fmt.Errorf("检测网卡失败: %w", err)
	}
	interfaceName := strings.TrimSpace(output)
	if interfaceName == "" {
		interfaceName = "eth0" // 默认值
	}
	
	// 生成路由 ID（使用 VIP 最后一位）
	routerID := getRouterID(cfg.Spec.HA.VIP)
	
	// 生成认证密码（使用集群名称）
	authPass := cfg.Metadata.Name
	if len(authPass) > 8 {
		authPass = authPass[:8]
	}
	
	keepalivedConfig := fmt.Sprintf(`# Keepalived configuration for Kubernetes HA
global_defs {
    router_id %s
}

# Health check script for API Server
vrrp_script check_apiserver {
    script "/etc/keepalived/check_apiserver.sh"
    interval 3
    weight -2
    fall 10
    rise 2
}

vrrp_instance VI_1 {
    state %s
    interface %s
    virtual_router_id %d
    priority %d
    advert_int 1
    
    authentication {
        auth_type PASS
        auth_pass %s
    }
    
    virtual_ipaddress {
        %s
    }
    
    track_script {
        check_apiserver
    }
}
`, node.Hostname, state, interfaceName, routerID, priority, authPass, cfg.Spec.HA.VIP)
	
	// 写入 Keepalived 配置
	cmd := fmt.Sprintf("cat > /etc/keepalived/keepalived.conf << 'EOF'\n%s\nEOF", keepalivedConfig)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("写入 Keepalived 配置失败: %w", err)
	}
	
	// 创建健康检查脚本
	checkScript := `#!/bin/bash
# Kubernetes API Server health check script

errorExit() {
    echo "*** $*" 1>&2
    exit 1
}

# Check if HAProxy is running
systemctl is-active --quiet haproxy || errorExit "HAProxy is not running"

# Check if API server is responding on localhost
curl --silent --max-time 2 --insecure https://localhost:6443/ -o /dev/null || errorExit "API Server is not responding"

exit 0
`
	
	// 写入健康检查脚本
	cmd = fmt.Sprintf("cat > /etc/keepalived/check_apiserver.sh << 'EOF'\n%s\nEOF", checkScript)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("写入健康检查脚本失败: %w", err)
	}
	
	// 设置执行权限
	if _, err := client.Execute("chmod +x /etc/keepalived/check_apiserver.sh"); err != nil {
		return fmt.Errorf("设置脚本权限失败: %w", err)
	}
	
	return nil
}

// getMasterNodes 获取所有 Master 节点
func getMasterNodes(cfg *config.ClusterConfig) []config.NodeConfig {
	var masters []config.NodeConfig
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "master" {
			masters = append(masters, node)
		}
	}
	return masters
}

// getRouterID 从 VIP 生成 Router ID
func getRouterID(vip string) int {
	parts := strings.Split(vip, ".")
	if len(parts) == 4 {
		var id int
		fmt.Sscanf(parts[3], "%d", &id)
		if id > 0 && id < 256 {
			return id
		}
	}
	return 51 // 默认值
}

// CheckHAStatus 检查 HA 状态
func CheckHAStatus(cfg *config.ClusterConfig) error {
	ui.Header("检查高可用状态")
	
	masterNodes := getMasterNodes(cfg)
	
	for i, node := range masterNodes {
		ui.Step(i+1, len(masterNodes), "检查节点: %s", node.Hostname)
		
		client, err := executor.NewSSHClientWithPassword(
			node.IP,
			node.SSH.Port,
			node.SSH.User,
			node.SSH.KeyFile,
			node.SSH.Password,
		)
		if err != nil {
			ui.Warning("连接失败: %v", err)
			continue
		}
		defer client.Close()
		
		// 检查 HAProxy 状态
		ui.SubStep("HAProxy 状态...")
		if _, err := client.Execute("systemctl is-active haproxy"); err != nil {
			ui.SubStepFailed()
			ui.Warning("HAProxy 未运行")
		} else {
			ui.SubStepDone()
		}
		
		// 检查 Keepalived 状态
		ui.SubStep("Keepalived 状态...")
		if _, err := client.Execute("systemctl is-active keepalived"); err != nil {
			ui.SubStepFailed()
			ui.Warning("Keepalived 未运行")
		} else {
			ui.SubStepDone()
		}
		
		// 检查 VIP
		ui.SubStep("检查 VIP...")
		output, err := client.Execute(fmt.Sprintf("ip addr show | grep '%s'", cfg.Spec.HA.VIP))
		if err == nil && strings.Contains(output, cfg.Spec.HA.VIP) {
			ui.SubStepDone()
			ui.Success("  → 当前节点持有 VIP")
		} else {
			ui.SubStepDone()
			ui.Info("  → 备用节点")
		}
	}
	
	return nil
}

