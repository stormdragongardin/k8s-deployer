package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long:  `显示 k8s-deployer 的版本信息`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("k8s-deployer v0.1.0")
		fmt.Println("Kubernetes 集群部署管理工具")
		fmt.Println("")
		fmt.Println("支持特性:")
		fmt.Println("  - Cilium 网络插件（替代 kube-proxy）")
		fmt.Println("  - GPU 节点支持")
		fmt.Println("  - Harbor 私有镜像仓库")
		fmt.Println("  - 高可用集群部署")
		fmt.Println("  - 系统性能优化")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

