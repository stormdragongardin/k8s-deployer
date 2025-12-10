package cluster

import (
	"fmt"
	"path/filepath"

	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

// configureGPU 配置 GPU 节点（完全离线）
func configureGPU(client *executor.SSHClient) error {
	ui.SubStep("上传 NVIDIA 驱动...")
	if err := uploadNvidiaDriverPackages(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("安装 NVIDIA 驱动...")
	if err := installNvidiaDriver(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("锁定驱动版本...")
	if err := lockDriverVersion(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("上传 nvidia-container-toolkit...")
	if err := uploadNvidiaContainerToolkit(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("安装 nvidia-container-toolkit...")
	if err := installNvidiaContainerToolkit(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("配置 containerd GPU 运行时...")
	if err := configureContainerdGPU(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("重启 containerd...")
	if _, err := client.Execute("systemctl restart containerd"); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	return nil
}

// uploadNvidiaDriverPackages 上传 NVIDIA 驱动离线包
func uploadNvidiaDriverPackages(client *executor.SSHClient) error {
	// 从本地 packages/gpu/ 目录上传
	gpuPkgDir := "packages/gpu"
	
	driverFiles := []string{
		"nvidia-driver-580-server-open_580.95.05-0ubuntu0.24.04.2_amd64.deb",
		"nvidia-dkms-580-server-open_580.95.05-0ubuntu0.24.04.2_amd64.deb",
		"nvidia-kernel-source-580-server-open_580.95.05-0ubuntu0.24.04.2_amd64.deb",
	}
	
	for _, file := range driverFiles {
		localPath := filepath.Join(gpuPkgDir, file)
		remotePath := fmt.Sprintf("/tmp/%s", file)
		
		if err := client.UploadFile(localPath, remotePath); err != nil {
			return fmt.Errorf("上传 %s 失败: %w", file, err)
		}
	}
	
	return nil
}

// installNvidiaDriver 安装 NVIDIA 驱动（使用离线 deb 包）
func installNvidiaDriver(client *executor.SSHClient) error {
	// 检查是否已安装驱动
	if _, err := client.Execute("nvidia-smi"); err == nil {
		ui.Info("  NVIDIA 驱动已安装")
		return nil
	}
	
	// 使用 dpkg 安装离线包
	installScript := `
		cd /tmp
		
		# 安装必要的依赖
		apt-get update
		apt-get install -y dkms build-essential linux-headers-$(uname -r)
		
		# 安装 NVIDIA 驱动离线包
		dpkg -i nvidia-kernel-source-580-server-open_*.deb || true
		dpkg -i nvidia-dkms-580-server-open_*.deb || true
		dpkg -i nvidia-driver-580-server-open_*.deb || true
		
		# 修复依赖
		apt-get install -f -y
		
		# 清理临时文件
		rm -f /tmp/nvidia-*.deb
		
		# 验证安装
		sleep 2
		if ! nvidia-smi > /dev/null 2>&1; then
			echo "警告: nvidia-smi 尚未可用，可能需要重启系统"
		fi
	`
	
	_, err := client.Execute(installScript)
	return err
}

// lockDriverVersion 锁定驱动版本
func lockDriverVersion(client *executor.SSHClient) error {
	lockScript := `
		# 标记软件包为 hold，防止自动升级
		apt-mark hold nvidia-driver-580-server-open
		apt-mark hold nvidia-dkms-580-server-open
		apt-mark hold nvidia-kernel-source-580-server-open
		
		echo "✓ NVIDIA 驱动 580-server-open 已锁定"
	`
	
	_, err := client.Execute(lockScript)
	return err
}

// uploadNvidiaContainerToolkit 上传 nvidia-container-toolkit 离线包
func uploadNvidiaContainerToolkit(client *executor.SSHClient) error {
	// 上传所有 toolkit 相关的 deb 包
	toolkitDir := "packages/gpu/nvidia-container-toolkit"
	
	debFiles := []string{
		"libnvidia-container1_1.18.0-1_amd64.deb",
		"libnvidia-container-tools_1.18.0-1_amd64.deb",
		"nvidia-container-toolkit-base_1.18.0-1_amd64.deb",
		"nvidia-container-toolkit_1.18.0-1_amd64.deb",
	}
	
	for _, file := range debFiles {
		localPath := filepath.Join(toolkitDir, file)
		remotePath := fmt.Sprintf("/tmp/%s", file)
		
		if err := client.UploadFile(localPath, remotePath); err != nil {
			return fmt.Errorf("上传 %s 失败: %w", file, err)
		}
	}
	
	return nil
}

// installNvidiaContainerToolkit 安装 nvidia-container-toolkit（使用离线 deb 包）
func installNvidiaContainerToolkit(client *executor.SSHClient) error {
	// 检查是否已安装
	if _, err := client.Execute("which nvidia-container-runtime"); err == nil {
		ui.Info("  nvidia-container-toolkit 已安装")
		return nil
	}
	
	installScript := `
		cd /tmp
		
		# 按顺序安装 deb 包（注意依赖关系）
		dpkg -i libnvidia-container1_1.18.0-1_amd64.deb || true
		dpkg -i libnvidia-container-tools_1.18.0-1_amd64.deb || true
		dpkg -i nvidia-container-toolkit-base_1.18.0-1_amd64.deb || true
		dpkg -i nvidia-container-toolkit_1.18.0-1_amd64.deb || true
		
		# 修复可能的依赖问题
		apt-get install -f -y
		
		# 清理临时文件
		rm -f /tmp/libnvidia-container*.deb /tmp/nvidia-container-toolkit*.deb
		
		# 验证安装
		which nvidia-container-runtime
		which nvidia-ctk
	`
	
	_, err := client.Execute(installScript)
	return err
}

// configureContainerdGPU 配置 containerd 使用 GPU 运行时
func configureContainerdGPU(client *executor.SSHClient) error {
	configScript := `
		# 使用 nvidia-ctk 自动配置 containerd
		nvidia-ctk runtime configure --runtime=containerd --set-as-default
		
		# 验证配置
		if grep -q "nvidia" /etc/containerd/config.toml; then
			echo "✓ containerd GPU 运行时配置完成"
		else
			echo "✗ containerd GPU 运行时配置失败"
			exit 1
		fi
	`
	
	_, err := client.Execute(configScript)
	return err
}

// LabelGPUNode 给 GPU 节点打标签
func LabelGPUNode(client *executor.SSHClient, nodeName string) error {
	cmd := fmt.Sprintf("kubectl label node %s gpu=on --overwrite", nodeName)
	_, err := client.Execute(cmd)
	if err != nil {
		return fmt.Errorf("标记 GPU 节点失败: %w", err)
	}
	
	ui.Success("已标记 GPU 节点: %s (gpu=on)", nodeName)
	return nil
}
