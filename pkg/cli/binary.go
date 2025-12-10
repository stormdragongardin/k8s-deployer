package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"stormdragon/k8s-deployer/pkg/binary"
	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/ui"
)

var binaryCmd = &cobra.Command{
	Use:   "binary",
	Short: "管理二进制文件缓存",
	Long:  `下载、列出和清理二进制文件缓存`,
}

var binaryDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "下载所有需要的二进制文件",
	Long:  `预下载 Kubernetes、containerd、Helm 等二进制文件到本地缓存`,
	Run: func(cmd *cobra.Command, args []string) {
		k8sVersion, _ := cmd.Flags().GetString("k8s-version")
		
		// 获取配置目录
		configDir, err := config.GetConfigDir()
		if err != nil {
			ui.Error("获取配置目录失败: %v", err)
			return
		}
		
		binariesDir := filepath.Join(configDir, "binaries")
		manager, err := binary.NewManager(binariesDir)
		if err != nil {
			ui.Error("创建二进制管理器失败: %v", err)
			return
		}
		
		// 下载所有文件
		if err := binary.PreDownloadAll(manager, k8sVersion); err != nil {
			ui.Error("下载失败: %v", err)
			return
		}
		
		ui.Success("所有二进制文件已下载到: %s", binariesDir)
	},
}

var binaryListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出已缓存的二进制文件",
	Long:  `显示本地缓存的所有二进制文件`,
	Run: func(cmd *cobra.Command, args []string) {
		// 获取配置目录
		configDir, err := config.GetConfigDir()
		if err != nil {
			ui.Error("获取配置目录失败: %v", err)
			return
		}
		
		binariesDir := filepath.Join(configDir, "binaries")
		manager, err := binary.NewManager(binariesDir)
		if err != nil {
			ui.Error("创建二进制管理器失败: %v", err)
			return
		}
		
		// 列出缓存文件
		cached, err := manager.ListCached()
		if err != nil {
			ui.Error("列出缓存失败: %v", err)
			return
		}
		
		if len(cached) == 0 {
			ui.Info("没有缓存的二进制文件")
			ui.Info("运行 'k8s-deployer binary download' 来下载")
			return
		}
		
		ui.Info("已缓存的二进制文件 (%d 个):", len(cached))
		for _, file := range cached {
			fmt.Printf("  - %s\n", file)
		}
	},
}

var binaryCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "清理二进制文件缓存",
	Long:  `删除所有缓存的二进制文件`,
	Run: func(cmd *cobra.Command, args []string) {
		// 获取配置目录
		configDir, err := config.GetConfigDir()
		if err != nil {
			ui.Error("获取配置目录失败: %v", err)
			return
		}
		
		binariesDir := filepath.Join(configDir, "binaries")
		manager, err := binary.NewManager(binariesDir)
		if err != nil {
			ui.Error("创建二进制管理器失败: %v", err)
			return
		}
		
		// 确认
		if !ui.WaitForConfirmation("确认清理所有缓存的二进制文件？") {
			ui.Info("已取消")
			return
		}
		
		// 清理缓存
		if err := manager.CleanCache(); err != nil {
			ui.Error("清理缓存失败: %v", err)
			return
		}
		
		ui.Success("缓存已清理")
	},
}

func init() {
	rootCmd.AddCommand(binaryCmd)
	binaryCmd.AddCommand(binaryDownloadCmd)
	binaryCmd.AddCommand(binaryListCmd)
	binaryCmd.AddCommand(binaryCleanCmd)
	
	// binary download 的 flags
	binaryDownloadCmd.Flags().String("k8s-version", "v1.34.2", "Kubernetes 版本")
}

