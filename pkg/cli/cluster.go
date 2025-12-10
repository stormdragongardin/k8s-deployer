package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"stormdragon/k8s-deployer/pkg/cluster"
	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/ui"
)

var (
	configFile      string
	skipSSHSetup    bool
	forceSSHSetup   bool
	autoConfirm     bool
	updateOnlyBGP   bool
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "部署 Kubernetes 集群",
	Long:  `部署和更新 Kubernetes 集群`,
}

var clusterCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建新的 Kubernetes 集群",
	Long: `根据配置文件创建一个新的 Kubernetes 集群

部署流程：
  1. 检查配置文件
  2. 自动配置 SSH 密钥（root 用户免密登录）
  3. 配置集群 Hosts 文件（节点互通）
  4. 系统优化（所有节点）
  5. 安装容器运行时和 K8s 组件（离线）
  6. 初始化 Master 节点
  7. 安装网络插件（Cilium）
  8. 加入 Worker 节点
  9. 配置 GPU 节点（如果有）
  10. 验证集群状态`,
	Example: `  # 创建集群（推荐）
  k8s-deployer cluster create -f cluster.yaml

  # 跳过 SSH 密钥配置（已经配置过）
  k8s-deployer cluster create -f cluster.yaml --skip-ssh-setup

  # 强制重新配置 SSH 密钥
  k8s-deployer cluster create -f cluster.yaml --force-ssh-setup`,
	RunE: runClusterCreate,
}

func runClusterCreate(cmd *cobra.Command, args []string) error {
	ui.Header("K8s Deployer - 集群部署工具")
	
	// 步骤 1: 加载配置
	ui.Info("加载配置文件: %s", configFile)
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
	
	ui.Success("配置加载成功: 集群 %s", cfg.Metadata.Name)
	ui.Info("  - Master 节点: %d 个", countMasterNodes(cfg))
	ui.Info("  - Worker 节点: %d 个", countWorkerNodes(cfg))
	ui.Info("  - GPU 节点: %d 个", countGPUNodes(cfg))
	ui.Info("  - Kubernetes 版本: %s", cfg.Spec.Version)
	ui.Info("")
	
	// 步骤 2: SSH 密钥配置（自动执行）
	if !skipSSHSetup {
		needsSetup, _ := checkSSHSetup(cfg)
		
		if needsSetup || forceSSHSetup {
			ui.Header("配置 SSH 密钥认证")
			ui.Info("检测到使用密码认证，自动配置 root 用户密钥登录...")
			ui.Info("")
			
			// 直接执行 SSH 密钥配置
			if err := cluster.SetupSSHKeys(cfg, forceSSHSetup); err != nil {
				ui.Error("SSH 密钥配置失败: %v", err)
				ui.Warn("您可以：")
				ui.Warn("  1. 使用 --skip-ssh-setup 跳过此步骤")
				ui.Warn("  2. 检查节点密码是否正确")
				ui.Warn("  3. 手动配置 SSH 密钥后重试")
				return err
			}
			
			ui.Success("SSH 密钥配置完成！")
			ui.Info("后续操作将使用 root 用户免密执行")
			ui.Info("")
			
			// 更新内存中的配置，使用 root + 密钥
			updateConfigToUseKeys(cfg)
		} else {
			ui.Info("SSH 密钥已配置，跳过")
		}
	}
	
	ui.Info("")
	
	// 步骤 3: 配置 Hosts 文件（节点互通）
	ui.Header("配置集群 Hosts 文件")
	ui.Info("Kubernetes 节点需要通过主机名互相解析...")
	ui.Info("")
	
	if err := cluster.SetupHostsFile(cfg); err != nil {
		ui.Error("配置 Hosts 文件失败: %v", err)
		ui.Warn("您可以手动配置 /etc/hosts 后重试")
		return err
	}
	
	ui.Info("")
	
	// 步骤 4: 开始部署集群
	if err := cluster.DeployCluster(cfg, autoConfirm); err != nil {
		ui.Error("集群部署失败: %v", err)
		return err
	}
	
	ui.Header("✓ 集群部署完成！")
	ui.Info("")
	ui.Info("验证集群:")
	ui.Info("  kubectl get nodes")
	ui.Info("  kubectl get pods -n kube-system")
	ui.Info("")
	
	return nil
}

// 辅助函数
func countMasterNodes(cfg *config.ClusterConfig) int {
	count := 0
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "master" {
			count++
		}
	}
	return count
}

func countWorkerNodes(cfg *config.ClusterConfig) int {
	count := 0
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "worker" {
			count++
		}
	}
	return count
}

func countGPUNodes(cfg *config.ClusterConfig) int {
	count := 0
	for _, node := range cfg.Spec.Nodes {
		if node.GPU {
			count++
		}
	}
	return count
}

func checkSSHSetup(cfg *config.ClusterConfig) (needsSetup bool, usingPassword bool) {
	for _, node := range cfg.Spec.Nodes {
		if node.SSH.Password != "" {
			return true, true
		}
	}
	return false, false
}

func updateConfigToUseKeys(cfg *config.ClusterConfig) {
	keyFile := "/root/.ssh/id_rsa"
	for i := range cfg.Spec.Nodes {
		if cfg.Spec.Nodes[i].SSH.Password != "" {
			cfg.Spec.Nodes[i].SSH.User = "root"
			cfg.Spec.Nodes[i].SSH.KeyFile = keyFile
			cfg.Spec.Nodes[i].SSH.Password = "" // 清除密码
		}
	}
}

var clusterUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "更新已部署的集群配置",
	Long:  `更新集群配置，支持增量更新（如添加 BGP、修改 Harbor 认证等）`,
	Example: `  # 更新集群配置
  k8s-deployer cluster update -f cluster.yaml

  # 只更新 BGP 配置
  k8s-deployer cluster update -f cluster.yaml --only-bgp
  
  # 自动确认所有变更
  k8s-deployer cluster update -f cluster.yaml -y`,
	RunE: runClusterUpdate,
}

func runClusterUpdate(cmd *cobra.Command, args []string) error {
	ui.Header("更新集群配置")

	// 加载新配置
	newCfg, err := config.LoadConfig(configFile)
	if err != nil {
		ui.Error("加载配置失败: %v", err)
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 执行更新
	return cluster.UpdateCluster(newCfg, updateOnlyBGP, autoConfirm)
}

func init() {
	rootCmd.AddCommand(clusterCmd)
	clusterCmd.AddCommand(clusterCreateCmd)
	clusterCmd.AddCommand(clusterUpdateCmd)

	// cluster create 的 flags
	clusterCreateCmd.Flags().StringVarP(&configFile, "config", "f", "", "集群配置文件路径 (必需)")
	clusterCreateCmd.Flags().BoolVar(&skipSSHSetup, "skip-ssh-setup", false, "跳过 SSH 密钥配置")
	clusterCreateCmd.Flags().BoolVar(&forceSSHSetup, "force-ssh-setup", false, "强制重新配置 SSH 密钥")
	clusterCreateCmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "自动确认所有提示")
	clusterCreateCmd.MarkFlagRequired("config")

	// cluster update 的 flags
	clusterUpdateCmd.Flags().StringVarP(&configFile, "config", "f", "", "集群配置文件路径 (必需)")
	clusterUpdateCmd.Flags().BoolVar(&updateOnlyBGP, "only-bgp", false, "仅更新 BGP 配置")
	clusterUpdateCmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "自动确认所有提示")
	clusterUpdateCmd.MarkFlagRequired("config")
}

