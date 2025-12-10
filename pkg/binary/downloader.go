package binary

import (
	"fmt"

	"stormdragon/k8s-deployer/pkg/ui"
)

// PreDownloadAll 预下载所有需要的二进制文件
func PreDownloadAll(manager *Manager, k8sVersion string) error {
	ui.Header("下载必需的二进制文件")
	
	allBinaries := []BinaryInfo{}
	
	// Kubernetes 组件
	k8sBinaries := GetKubernetesVersion(k8sVersion)
	allBinaries = append(allBinaries, k8sBinaries...)
	
	// containerd
	allBinaries = append(allBinaries, GetContainerdInfo("1.7.10"))
	
	// Helm
	allBinaries = append(allBinaries, GetHelmInfo("3.13.3"))
	
	ui.Info("需要下载 %d 个文件", len(allBinaries))
	
	for i, binary := range allBinaries {
		ui.Step(i+1, len(allBinaries), fmt.Sprintf("下载 %s %s", binary.Name, binary.Version))
		
		_, err := manager.GetBinaryPath(binary)
		if err != nil {
			return fmt.Errorf("下载 %s 失败: %w", binary.Name, err)
		}
	}
	
	ui.Success("所有二进制文件已准备完成！")
	return nil
}

// DownloadKubernetesComponents 下载 Kubernetes 组件
func DownloadKubernetesComponents(manager *Manager, version string) (map[string]string, error) {
	binaries := GetKubernetesVersion(version)
	paths := make(map[string]string)
	
	for _, binary := range binaries {
		path, err := manager.GetBinaryPath(binary)
		if err != nil {
			return nil, err
		}
		paths[binary.Name] = path
	}
	
	return paths, nil
}

// DownloadContainerd 下载 containerd
func DownloadContainerd(manager *Manager, version string) (string, error) {
	binary := GetContainerdInfo(version)
	return manager.GetBinaryPath(binary)
}

// DownloadHelm 下载 Helm
func DownloadHelm(manager *Manager, version string) (string, error) {
	binary := GetHelmInfo(version)
	return manager.GetBinaryPath(binary)
}

// GetDefaultVersions 获取推荐的默认版本
func GetDefaultVersions() map[string]string {
	return map[string]string{
		"kubernetes": "v1.34.2",
		"containerd": "1.7.10",
		"helm":       "3.13.3",
	}
}

