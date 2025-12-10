package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "k8s-deployer",
	Short: "Kubernetes 集群部署管理工具",
	Long: `k8s-deployer 是一个用于部署和管理 Kubernetes 集群的 CLI 工具。

支持特性:
  - 高可用集群部署（多 Master + Worker）
  - GPU 节点支持
  - Cilium 网络插件（替代 kube-proxy）
  - Harbor 私有镜像仓库集成
  - 系统优化和性能调优
  - 节点动态管理`,
	Version: "0.1.0",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// 添加全局 flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "详细输出模式")
	rootCmd.PersistentFlags().String("config-dir", "", "配置目录 (默认: ~/.k8s-deployer)")
}

