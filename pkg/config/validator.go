package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

// ValidateConfig 验证集群配置
func ValidateConfig(cfg *ClusterConfig) error {
	// 验证基础信息
	if err := validateMetadata(cfg); err != nil {
		return err
	}

	// 验证集群规格
	if err := validateSpec(cfg); err != nil {
		return err
	}

	// 验证节点配置
	if err := validateNodes(cfg); err != nil {
		return err
	}

	// 验证 BGP 配置
	if err := validateBGP(&cfg.Spec.BGP); err != nil {
		return err
	}

	return nil
}

// validateMetadata 验证元数据
func validateMetadata(cfg *ClusterConfig) error {
	if cfg.APIVersion == "" {
		return fmt.Errorf("apiVersion 不能为空")
	}

	if cfg.Kind == "" {
		return fmt.Errorf("kind 不能为空")
	}

	if cfg.Metadata.Name == "" {
		return fmt.Errorf("metadata.name 不能为空")
	}

	// 验证集群名称格式（只允许小写字母、数字和连字符）
	nameRegex := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !nameRegex.MatchString(cfg.Metadata.Name) {
		return fmt.Errorf("集群名称只能包含小写字母、数字和连字符")
	}

	return nil
}

// validateSpec 验证集群规格
func validateSpec(cfg *ClusterConfig) error {
	// 验证 Kubernetes 版本
	if cfg.Spec.Version == "" {
		return fmt.Errorf("spec.version 不能为空")
	}
	versionRegex := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	if !versionRegex.MatchString(cfg.Spec.Version) {
		return fmt.Errorf("Kubernetes 版本格式不正确，应为 vX.Y.Z 格式，如: v1.34.2")
	}

	// 验证镜像仓库
	if cfg.Spec.ImageRepository == "" {
		return fmt.Errorf("spec.imageRepository 不能为空")
	}

	// 验证网络配置
	if err := validateNetworking(&cfg.Spec.Networking); err != nil {
		return err
	}

	// 验证高可用配置
	if err := validateHA(cfg); err != nil {
		return err
	}

	return nil
}

// validateNetworking 验证网络配置
func validateNetworking(net *NetworkConfig) error {
	// 验证 Pod 网段
	if net.PodSubnet == "" {
		return fmt.Errorf("spec.networking.podSubnet 不能为空")
	}
	if _, _, err := parseAndValidateCIDR(net.PodSubnet); err != nil {
		return fmt.Errorf("Pod 网段格式不正确: %w", err)
	}

	// 验证 Service 网段
	if net.ServiceSubnet == "" {
		return fmt.Errorf("spec.networking.serviceSubnet 不能为空")
	}
	if _, _, err := parseAndValidateCIDR(net.ServiceSubnet); err != nil {
		return fmt.Errorf("Service 网段格式不正确: %w", err)
	}

	// 验证网段不重叠
	podIP, podNet, _ := parseAndValidateCIDR(net.PodSubnet)
	svcIP, svcNet, _ := parseAndValidateCIDR(net.ServiceSubnet)
	if podNet.Contains(svcIP) || svcNet.Contains(podIP) {
		return fmt.Errorf("Pod 网段和 Service 网段不能重叠")
	}

	return nil
}

// validateHA 验证高可用配置
func validateHA(cfg *ClusterConfig) error {
	if !cfg.Spec.HA.Enabled {
		return nil
	}

	// 检查 Master 节点数量
	masterCount := 0
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "master" {
			masterCount++
		}
	}

	if masterCount < 3 {
		return fmt.Errorf("高可用模式至少需要 3 个 Master 节点，当前只有 %d 个", masterCount)
	}

	// 验证 VIP
	if cfg.Spec.HA.VIP == "" {
		return fmt.Errorf("启用高可用模式时，spec.ha.vip 不能为空")
	}

	if net.ParseIP(cfg.Spec.HA.VIP) == nil {
		return fmt.Errorf("VIP 地址格式不正确: %s", cfg.Spec.HA.VIP)
	}

	return nil
}

// validateNodes 验证节点配置
func validateNodes(cfg *ClusterConfig) error {
	if len(cfg.Spec.Nodes) == 0 {
		return fmt.Errorf("至少需要配置一个节点")
	}

	// 检查是否至少有一个 Master 节点
	hasMaster := false
	ipMap := make(map[string]bool)
	hostnameMap := make(map[string]bool)

	for i, node := range cfg.Spec.Nodes {
		// 验证角色
		if node.Role != "master" && node.Role != "worker" {
			return fmt.Errorf("节点 %d 的角色不正确，只能是 'master' 或 'worker'", i)
		}

		if node.Role == "master" {
			hasMaster = true
		}

		// 验证 IP
		if node.IP == "" {
			return fmt.Errorf("节点 %d 的 IP 地址不能为空", i)
		}
		if net.ParseIP(node.IP) == nil {
			return fmt.Errorf("节点 %d 的 IP 地址格式不正确: %s", i, node.IP)
		}

		// 检查 IP 重复
		if ipMap[node.IP] {
			return fmt.Errorf("节点 IP 地址重复: %s", node.IP)
		}
		ipMap[node.IP] = true

		// 验证主机名
		if node.Hostname == "" {
			return fmt.Errorf("节点 %d 的主机名不能为空", i)
		}

		// 检查主机名重复
		if hostnameMap[node.Hostname] {
			return fmt.Errorf("节点主机名重复: %s", node.Hostname)
		}
		hostnameMap[node.Hostname] = true

		// 验证主机名格式
		hostnameRegex := regexp.MustCompile(`^[a-z0-9-]+$`)
		if !hostnameRegex.MatchString(node.Hostname) {
			return fmt.Errorf("节点 %d 的主机名格式不正确（只能包含小写字母、数字和连字符）: %s", i, node.Hostname)
		}

		// 验证 SSH 配置
		if err := validateSSH(&node.SSH, i); err != nil {
			return err
		}

		// Master 节点不应该是 GPU 节点
		if node.Role == "master" && node.GPU {
			return fmt.Errorf("节点 %d: Master 节点不应该配置为 GPU 节点", i)
		}
	}

	if !hasMaster {
		return fmt.Errorf("至少需要配置一个 Master 节点")
	}

	return nil
}

// validateSSH 验证 SSH 配置
func validateSSH(ssh *SSHConfig, nodeIndex int) error {
	// 验证用户名
	if ssh.User == "" {
		return fmt.Errorf("节点 %d 的 SSH 用户名不能为空", nodeIndex)
	}

	// 验证端口
	if ssh.Port <= 0 || ssh.Port > 65535 {
		return fmt.Errorf("节点 %d 的 SSH 端口不正确: %d", nodeIndex, ssh.Port)
	}

	// 验证认证方式（必须提供密钥或密码）
	if ssh.KeyFile == "" && ssh.Password == "" {
		return fmt.Errorf("节点 %d 必须提供 SSH 密钥文件或密码", nodeIndex)
	}

	// 如果提供了密钥文件，检查文件是否存在
	if ssh.KeyFile != "" {
		keyPath := expandPath(ssh.KeyFile)
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			return fmt.Errorf("节点 %d 的 SSH 密钥文件不存在: %s", nodeIndex, keyPath)
		}
	}

	// 如果同时提供密钥和密码，给出警告（但不报错）
	if ssh.KeyFile != "" && ssh.Password != "" {
		fmt.Printf("警告: 节点 %d 同时配置了密钥和密码，将优先使用密钥认证\n", nodeIndex)
	}

	return nil
}

// parseAndValidateCIDR 解析并验证 CIDR
func parseAndValidateCIDR(cidr string) (net.IP, *net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("CIDR 格式不正确: %s", cidr)
	}
	return ip, ipNet, nil
}

// expandPath 展开路径（处理 ~ 等）
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return strings.Replace(path, "~", home, 1)
		}
	}
	return path
}

// validateBGP 验证 BGP 配置
func validateBGP(bgp *BGPConfig) error {
	if !bgp.Enabled {
		return nil
	}

	// 验证 AS 号范围
	if bgp.LocalASN < 1 || bgp.LocalASN > 65535 {
		return fmt.Errorf("LocalASN 必须在 1-65535 范围内")
	}

	// 验证至少有一个 Peer
	if len(bgp.Peers) == 0 {
		return fmt.Errorf("启用 BGP 时至少需要配置一个 Peer")
	}

	// 验证每个 Peer
	for i, peer := range bgp.Peers {
		if net.ParseIP(peer.PeerAddress) == nil {
			return fmt.Errorf("Peer %d 的 IP 地址格式不正确", i)
		}
		if peer.PeerASN < 1 || peer.PeerASN > 65535 {
			return fmt.Errorf("Peer %d 的 AS 号必须在 1-65535 范围内", i)
		}
	}

	// 验证 LoadBalancer IP 池
	if len(bgp.LoadBalancerIPs) == 0 {
		return fmt.Errorf("启用 BGP 时需要配置 LoadBalancer IP 池")
	}

	for i, ip := range bgp.LoadBalancerIPs {
		// 支持三种格式：单个 IP、CIDR、IP 范围
		if strings.Contains(ip, "-") {
			// IP 范围格式: 10.0.4.150-10.0.4.199
			if err := validateIPRange(ip); err != nil {
				return fmt.Errorf("LoadBalancer IP %d 范围格式不正确: %w", i, err)
			}
		} else if strings.Contains(ip, "/") {
			// CIDR 格式
			if _, _, err := net.ParseCIDR(ip); err != nil {
				return fmt.Errorf("LoadBalancer IP %d CIDR 格式不正确: %w", i, err)
			}
		} else {
			// 单个 IP
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("LoadBalancer IP %d 格式不正确", i)
			}
		}
	}

	return nil
}

// validateIPRange 验证 IP 范围格式
func validateIPRange(ipRange string) error {
	parts := strings.Split(ipRange, "-")
	if len(parts) != 2 {
		return fmt.Errorf("IP 范围格式应为: 起始IP-结束IP，例如: 10.0.4.150-10.0.4.199")
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	if startIP == nil {
		return fmt.Errorf("起始 IP 格式不正确: %s", parts[0])
	}
	if endIP == nil {
		return fmt.Errorf("结束 IP 格式不正确: %s", parts[1])
	}

	// 转换为 IPv4
	startIPv4 := startIP.To4()
	endIPv4 := endIP.To4()
	if startIPv4 == nil || endIPv4 == nil {
		return fmt.Errorf("目前仅支持 IPv4 地址范围")
	}

	// 验证起始 IP <= 结束 IP
	if ipToInt(startIPv4) > ipToInt(endIPv4) {
		return fmt.Errorf("起始 IP (%s) 必须小于或等于结束 IP (%s)", parts[0], parts[1])
	}

	return nil
}

// ipToInt 将 IP 转换为整数（用于比较）
func ipToInt(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// ValidateImmutableFields 验证不可变字段（用于更新时检查）
func ValidateImmutableFields(oldCfg, newCfg *ClusterConfig) error {
	var errors []string

	// 1. 集群名称不可变
	if oldCfg.Metadata.Name != newCfg.Metadata.Name {
		errors = append(errors, "集群名称不可修改")
	}

	// 2. Pod 网段不可变
	if oldCfg.Spec.Networking.PodSubnet != newCfg.Spec.Networking.PodSubnet {
		errors = append(errors, fmt.Sprintf(
			"Pod 网段不可修改 (当前: %s, 尝试修改为: %s)",
			oldCfg.Spec.Networking.PodSubnet,
			newCfg.Spec.Networking.PodSubnet,
		))
	}

	// 3. Service 网段不可变
	if oldCfg.Spec.Networking.ServiceSubnet != newCfg.Spec.Networking.ServiceSubnet {
		errors = append(errors, fmt.Sprintf(
			"Service 网段不可修改 (当前: %s, 尝试修改为: %s)",
			oldCfg.Spec.Networking.ServiceSubnet,
			newCfg.Spec.Networking.ServiceSubnet,
		))
	}

	// 4. Kubernetes 版本不可直接修改（需要专门的升级流程）
	if oldCfg.Spec.Version != newCfg.Spec.Version {
		errors = append(errors, fmt.Sprintf(
			"Kubernetes 版本不可通过 update 命令修改，请使用 upgrade 命令 (当前: %s, 尝试修改为: %s)",
			oldCfg.Spec.Version,
			newCfg.Spec.Version,
		))
	}

	if len(errors) > 0 {
		return fmt.Errorf("检测到不可变配置被修改:\n  - %s",
			strings.Join(errors, "\n  - "))
	}

	return nil
}

