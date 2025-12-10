package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"stormdragon/k8s-deployer/pkg/cluster"
	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/ui"
)

var (
	initForceNew bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化部署环境",
	Long:  "初始化部署环境，包括 SSH 密钥配置等",
}

// init ssh-key 命令
var sshKeyCmd = &cobra.Command{
	Use:   "ssh-key",
	Short: "配置 SSH 密钥认证",
	Long: `配置 SSH 密钥认证
	
该命令会：
1. 检查或生成本地 SSH 密钥对（~/.ssh/id_rsa）
2. 使用密码连接到所有节点
3. 将公钥添加到 root 用户的 authorized_keys
4. 配置 SSH 服务允许 root 登录和密钥认证
5. 验证配置是否成功

完成后，可以更新配置文件使用密钥认证：
  ssh:
    user: root
    port: 22
    keyFile: ~/.ssh/id_rsa`,
	Example: `  # 为集群配置 SSH 密钥
  k8s-deployer init ssh-key -f cluster.yaml
  
  # 强制生成新密钥
  k8s-deployer init ssh-key -f cluster.yaml --force-new`,
	RunE: runInitSSHKey,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.AddCommand(sshKeyCmd)

	// ssh-key 命令的 flags
	sshKeyCmd.Flags().StringVarP(&configFile, "config", "f", "", "集群配置文件路径")
	sshKeyCmd.Flags().BoolVar(&initForceNew, "force-new", false, "强制生成新的 SSH 密钥")
	sshKeyCmd.MarkFlagRequired("config")
}

func runInitSSHKey(cmd *cobra.Command, args []string) error {
	// 加载配置
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		ui.Error("加载配置文件失败: %v", err)
		return err
	}

	// 验证配置
	if err := config.ValidateConfig(cfg); err != nil {
		ui.Error("配置验证失败: %v", err)
		return err
	}

	// 检查是否所有节点都配置了密码
	for _, node := range cfg.Spec.Nodes {
		if node.SSH.Password == "" && node.SSH.KeyFile == "" {
			ui.Error("节点 %s 未配置密码或密钥", node.Hostname)
			return fmt.Errorf("节点 %s 必须配置 password 或 keyFile", node.Hostname)
		}
	}

	// 执行 SSH 密钥配置
	if err := cluster.SetupSSHKeys(cfg, initForceNew); err != nil {
		ui.Error("SSH 密钥配置失败: %v", err)
		return err
	}

	ui.Header("✓ 配置完成！")
	ui.Info("")
	ui.Info("下一步：更新配置文件以使用密钥认证")
	ui.Info("")
	ui.Info("将配置文件中的 SSH 配置修改为：")
	ui.Info("  ssh:")
	ui.Info("    user: root")
	ui.Info("    port: 22")
	ui.Info("    keyFile: ~/.ssh/id_rsa")
	ui.Info("    # password: \"...\"  # 可以删除或注释掉")
	ui.Info("")

	return nil
}

