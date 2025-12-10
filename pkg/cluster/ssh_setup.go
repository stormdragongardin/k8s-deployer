package cluster

import (
	"fmt"
	"os"
	"path/filepath"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

// SetupSSHKeys 为所有节点配置 SSH 密钥认证
func SetupSSHKeys(cfg *config.ClusterConfig, forceNew bool) error {
	ui.Header("配置 SSH 密钥认证")
	
	// 步骤 1: 检查或生成本地 SSH 密钥
	keyPath, pubKeyContent, err := ensureSSHKey(forceNew)
	if err != nil {
		return err
	}
	
	ui.Success("SSH 密钥准备完成: %s", keyPath)
	
	// 步骤 2: 将公钥分发到所有节点
	ui.Info("开始分发公钥到所有节点...")
	
	for i, node := range cfg.Spec.Nodes {
		ui.Step(i+1, len(cfg.Spec.Nodes), "配置节点: %s (%s)", node.Hostname, node.IP)
		
		if err := setupNodeSSHKey(node, pubKeyContent); err != nil {
			ui.Error("配置节点 %s 失败: %v", node.Hostname, err)
			return err
		}
		
		ui.Success("节点 %s 配置完成", node.Hostname)
	}
	
	ui.Header("✓ SSH 密钥配置完成！")
	ui.Info("现在可以更新配置文件，移除密码，使用密钥认证：")
	ui.Info("  keyFile: %s", keyPath)
	ui.Info("  user: root  # 已配置 root 用户免密登录")
	
	return nil
}

// ensureSSHKey 确保本地有 SSH 密钥
func ensureSSHKey(forceNew bool) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	
	sshDir := filepath.Join(homeDir, ".ssh")
	privateKeyPath := filepath.Join(sshDir, "id_rsa")
	publicKeyPath := filepath.Join(sshDir, "id_rsa.pub")
	
	// 检查是否已存在完整的密钥对
	if !forceNew {
		privExists := false
		pubExists := false
		
		if _, err := os.Stat(privateKeyPath); err == nil {
			privExists = true
		}
		if _, err := os.Stat(publicKeyPath); err == nil {
			pubExists = true
		}
		
		// 只有当私钥和公钥都存在时才使用现有密钥
		if privExists && pubExists {
			ui.Info("使用现有 SSH 密钥: %s", privateKeyPath)
			
			// 读取公钥
			pubKey, err := os.ReadFile(publicKeyPath)
			if err != nil {
				return "", "", fmt.Errorf("读取公钥失败: %w", err)
			}
			
			return privateKeyPath, string(pubKey), nil
		} else if privExists || pubExists {
			// 如果只有一个文件存在，提示并重新生成
			ui.Warn("检测到不完整的密钥对，将重新生成")
			if privExists {
				os.Remove(privateKeyPath)
			}
			if pubExists {
				os.Remove(publicKeyPath)
			}
		}
	}
	
	// 生成新密钥
	ui.Info("生成新的 SSH 密钥...")
	
	// 确保 .ssh 目录存在
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", "", fmt.Errorf("创建 .ssh 目录失败: %w", err)
	}
	
	// 如果是强制生成，先删除旧密钥
	if forceNew {
		os.Remove(privateKeyPath)
		os.Remove(publicKeyPath)
	}
	
	// 使用 ssh-keygen 生成密钥
	cmd := fmt.Sprintf("ssh-keygen -t rsa -b 4096 -f %s -N '' -C 'k8s-deployer@%s'", 
		privateKeyPath, 
		os.Getenv("HOSTNAME"))
	
	if err := executeLocalCommand(cmd); err != nil {
		return "", "", fmt.Errorf("生成 SSH 密钥失败: %w", err)
	}
	
	// 读取公钥
	pubKey, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", "", fmt.Errorf("读取公钥失败: %w", err)
	}
	
	ui.Success("SSH 密钥已生成")
	return privateKeyPath, string(pubKey), nil
}

// setupNodeSSHKey 为单个节点配置 SSH 密钥
func setupNodeSSHKey(node config.NodeConfig, pubKey string) error {
	// 使用密码连接（第一次）
	client, err := executor.NewSSHClientWithPassword(
		node.IP,
		node.SSH.Port,
		node.SSH.User,
		"", // 不使用密钥
		node.SSH.Password,
	)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()
	
	ui.SubStep("切换到 root 用户...")
	
	// 配置脚本（使用 sudo -S 从标准输入读取密码）
	setupScript := fmt.Sprintf(`
		# 使用 sudo -S 从标准输入读取密码
		echo '%s' | sudo -S bash -c '
			# 创建 root 的 .ssh 目录
			mkdir -p /root/.ssh
			chmod 700 /root/.ssh
			
			# 添加公钥到 authorized_keys
			echo "%s" >> /root/.ssh/authorized_keys
			
			# 去重（如果公钥已存在）
			sort -u /root/.ssh/authorized_keys -o /root/.ssh/authorized_keys
			
			# 设置正确的权限
			chmod 600 /root/.ssh/authorized_keys
			chown root:root /root/.ssh/authorized_keys
			
			# 确保 SSH 配置允许 root 登录和公钥认证
			sed -i "s/^#*PermitRootLogin.*/PermitRootLogin yes/" /etc/ssh/sshd_config
			sed -i "s/^#*PubkeyAuthentication.*/PubkeyAuthentication yes/" /etc/ssh/sshd_config
			sed -i "s/^#*AuthorizedKeysFile.*/AuthorizedKeysFile .ssh\/authorized_keys/" /etc/ssh/sshd_config
			
			# 重启 SSH 服务
			systemctl restart sshd || systemctl restart ssh || service ssh restart
			
			echo "SSH key configured for root"
		'
	`, node.SSH.Password, pubKey)
	
	_, err = client.Execute(setupScript)
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("配置 SSH 密钥失败: %w", err)
	}
	ui.SubStepDone()
	
	// 验证配置（尝试用 root 连接）
	ui.SubStep("验证 root 用户 SSH 密钥...")
	
	homeDir, _ := os.UserHomeDir()
	keyPath := filepath.Join(homeDir, ".ssh", "id_rsa")
	
	testClient, err := executor.NewSSHClient(node.IP, node.SSH.Port, "root", keyPath)
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("验证失败: %w", err)
	}
	defer testClient.Close()
	
	_, err = testClient.Execute("whoami")
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("验证失败: %w", err)
	}
	ui.SubStepDone()
	
	return nil
}

// executeLocalCommand 执行本地命令
func executeLocalCommand(cmd string) error {
	_, err := executor.ExecuteLocalCommand(cmd)
	return err
}

