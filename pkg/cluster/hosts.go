package cluster

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

// SetupHostsFile 配置所有节点的 /etc/hosts 文件
func SetupHostsFile(cfg *config.ClusterConfig) error {
	ui.Header("配置集群 Hosts 文件")
	
	// 生成 hosts 条目
	hostsEntries := generateHostsEntries(cfg)
	
	ui.Info("将添加以下 hosts 条目到所有节点：")
	for _, entry := range hostsEntries {
		ui.Info("  %s", entry)
	}
	ui.Info("")
	
	// 步骤 1: 配置本地 hosts（运行 k8s-deployer 的机器）
	ui.SubStep("配置本地 hosts 文件...")
	if err := updateLocalHostsFile(hostsEntries, cfg.Metadata.Name); err != nil {
		ui.SubStepFailed()
		ui.Warn("本地 hosts 配置失败（非致命）: %v", err)
		ui.Info("您可以手动添加 hosts 条目")
	} else {
		ui.SubStepDone()
	}
	
	// 步骤 2: 为每个节点配置 hosts
	for i, node := range cfg.Spec.Nodes {
		ui.Step(i+1, len(cfg.Spec.Nodes), "配置节点: %s (%s)", node.Hostname, node.IP)
		
		// 建立 SSH 连接
		client, err := executor.NewSSHClientWithPassword(
			node.IP,
			node.SSH.Port,
			node.SSH.User,
			node.SSH.KeyFile,
			node.SSH.Password,
		)
		if err != nil {
			ui.Error("SSH 连接失败: %v", err)
			return err
		}
		defer client.Close()
		
		// 更新 hosts 文件
		if err := updateRemoteHostsFile(client, hostsEntries, cfg.Metadata.Name); err != nil {
			ui.Error("更新 hosts 文件失败: %v", err)
			return err
		}
		
		ui.Success("节点 %s 配置完成", node.Hostname)
	}
	
	ui.Header("✓ Hosts 文件配置完成")
	return nil
}

// generateHostsEntries 生成 hosts 条目
func generateHostsEntries(cfg *config.ClusterConfig) []string {
	var entries []string
	
	for _, node := range cfg.Spec.Nodes {
		// 格式: IP  hostname
		entry := fmt.Sprintf("%s\t%s", node.IP, node.Hostname)
		entries = append(entries, entry)
	}
	
	return entries
}

// updateLocalHostsFile 更新本地（运行 k8s-deployer 的机器）的 hosts 文件
func updateLocalHostsFile(entries []string, clusterName string) error {
	// 检查本地 hostname
	localHostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("获取本地主机名失败: %w", err)
	}
	
	// 检查本地机器是否在集群节点中
	isClusterNode := false
	for _, entry := range entries {
		if strings.Contains(entry, localHostname) {
			isClusterNode = true
			break
		}
	}
	
	if isClusterNode {
		ui.Info("  本地机器是集群节点，将由远程更新处理")
		return nil
	}
	
	// 读取当前 hosts 文件
	hostsData, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return fmt.Errorf("读取 /etc/hosts 失败: %w", err)
	}
	
	currentHosts := string(hostsData)
	marker := fmt.Sprintf("# === %s Cluster Hosts (Managed by k8s-deployer) ===", clusterName)
	
	// 检查是否已存在相同的集群配置
	if strings.Contains(currentHosts, marker) {
		ui.Info("  本地 hosts 已包含集群配置，更新中...")
		// 删除旧配置
		lines := strings.Split(currentHosts, "\n")
		var newLines []string
		inClusterSection := false
		endMarker := fmt.Sprintf("# === End of %s Cluster Hosts ===", clusterName)
		
		for _, line := range lines {
			if strings.TrimSpace(line) == marker {
				inClusterSection = true
				continue
			}
			if strings.TrimSpace(line) == endMarker {
				inClusterSection = false
				continue
			}
			if !inClusterSection {
				newLines = append(newLines, line)
			}
		}
		currentHosts = strings.Join(newLines, "\n")
	}
	
	// 生成新的 hosts 内容
	hostsContent := strings.Join(entries, "\n")
	newHosts := fmt.Sprintf("%s\n%s\n%s\n# === End of %s Cluster Hosts ===\n",
		currentHosts, marker, hostsContent, clusterName)
	
	// 备份原文件
	backupFile := fmt.Sprintf("/etc/hosts.backup.k8s-deployer.%s", clusterName)
	if err := os.WriteFile(backupFile, hostsData, 0644); err != nil {
		return fmt.Errorf("备份 hosts 文件失败: %w", err)
	}
	
	// 写入新的 hosts 文件（需要 root 权限）
	tmpFile := fmt.Sprintf("/tmp/hosts.k8s-deployer.%s", clusterName)
	if err := os.WriteFile(tmpFile, []byte(newHosts), 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	
	// 使用 sudo 复制到 /etc/hosts
	cmd := exec.Command("sudo", "cp", tmpFile, "/etc/hosts")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("更新 /etc/hosts 失败: %s, %w", string(output), err)
	}
	
	// 清理临时文件
	os.Remove(tmpFile)
	
	ui.Info("  本地 hosts 备份: %s", backupFile)
	return nil
}

// updateRemoteHostsFile 更新远程节点的 /etc/hosts 文件
func updateRemoteHostsFile(client *executor.SSHClient, entries []string, clusterName string) error {
	ui.SubStep("备份原 hosts 文件...")
	
	// 备份原 hosts 文件
	backupCmd := "cp /etc/hosts /etc/hosts.backup.$(date +%Y%m%d%H%M%S)"
	if _, err := client.Execute(backupCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("备份 hosts 文件失败: %w", err)
	}
	ui.SubStepDone()
	
	ui.SubStep("更新 hosts 条目...")
	
	// 生成要添加的内容
	marker := fmt.Sprintf("# === %s Cluster Hosts (Managed by k8s-deployer) ===", clusterName)
	endMarker := fmt.Sprintf("# === End of %s Cluster Hosts ===", clusterName)
	hostsContent := strings.Join(entries, "\n")
	
	// 构建智能更新脚本
	// 1. 删除旧的集群 hosts 条目（如果存在）
	// 2. 添加新的集群 hosts 条目
	// 3. 保留用户手动维护的其他条目
	updateScript := fmt.Sprintf(`
		# 创建临时文件
		TMP_HOSTS="/tmp/hosts.tmp.$$"
		
		# 读取当前 hosts，过滤掉旧的集群配置
		awk '
			/^# === %s Cluster Hosts/ { skip=1; next }
			/^# === End of %s Cluster Hosts/ { skip=0; next }
			!skip { print }
		' /etc/hosts > "$TMP_HOSTS"
		
		# 添加新的集群 hosts 条目
		cat >> "$TMP_HOSTS" << 'HOSTS_EOF'

%s
%s
%s
HOSTS_EOF
		
		# 原子性替换（避免并发问题）
		mv "$TMP_HOSTS" /etc/hosts
		chmod 644 /etc/hosts
	`, clusterName, clusterName, marker, hostsContent, endMarker)
	
	if _, err := client.Execute(updateScript); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("更新 hosts 文件失败: %w", err)
	}
	ui.SubStepDone()
	
	// 验证 hosts 文件
	ui.SubStep("验证 hosts 文件...")
	output, err := client.Execute("cat /etc/hosts | tail -30")
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("验证失败: %w", err)
	}
	
	// 检查是否包含集群标记
	if !strings.Contains(output, marker) {
		ui.SubStepFailed()
		return fmt.Errorf("hosts 文件未正确更新")
	}
	ui.SubStepDone()
	
	return nil
}

// TestHostsResolution 测试主机名解析
func TestHostsResolution(client *executor.SSHClient, targetHostname string) error {
	ui.SubStep("测试解析 %s...", targetHostname)
	
	// 使用 getent 测试主机名解析
	testCmd := fmt.Sprintf("getent hosts %s", targetHostname)
	output, err := client.Execute(testCmd)
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("无法解析主机名 %s: %w", targetHostname, err)
	}
	
	if output == "" {
		ui.SubStepFailed()
		return fmt.Errorf("主机名 %s 解析为空", targetHostname)
	}
	
	ui.SubStepDone()
	return nil
}

