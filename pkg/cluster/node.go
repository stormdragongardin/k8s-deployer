package cluster

import (
	"fmt"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/kubeadm"
	"stormdragon/k8s-deployer/pkg/ui"
)

// AddNode 添加节点到集群
func AddNode(masterIP string, masterSSHConfig config.SSHConfig, newNode *config.NodeConfig, imageRepo, controlPlaneEndpoint, k8sVersion string) error {
	ui.Header(fmt.Sprintf("添加节点: %s (%s)", newNode.Hostname, newNode.IP))
	
	// 步骤 1: 准备新节点
	ui.Step(1, 3, "准备节点环境")
	if err := PrepareNode(newNode, imageRepo, k8sVersion); err != nil {
		return err
	}
	
	// 步骤 2: 获取 join 信息
	ui.Step(2, 3, "获取集群 join 信息")
	
	masterClient, err := executor.NewSSHClient(masterIP, masterSSHConfig.Port, masterSSHConfig.User, masterSSHConfig.KeyFile)
	if err != nil {
		return fmt.Errorf("连接 master 节点失败: %w", err)
	}
	defer masterClient.Close()
	
	isMaster := (newNode.Role == "master")
	joinInfo, err := kubeadm.GetJoinInfo(masterClient, controlPlaneEndpoint, isMaster)
	if err != nil {
		return err
	}
	
	// 步骤 3: 加入集群
	ui.Step(3, 3, "加入集群")
	
	nodeClient, err := executor.NewSSHClient(newNode.IP, newNode.SSH.Port, newNode.SSH.User, newNode.SSH.KeyFile)
	if err != nil {
		return fmt.Errorf("连接新节点失败: %w", err)
	}
	defer nodeClient.Close()
	
	var joinCmd string
	if isMaster {
		joinCmd = kubeadm.GenerateMasterJoinCommand(joinInfo)
		ui.Info("加入 Master 节点...")
	} else {
		joinCmd = kubeadm.GenerateWorkerJoinCommand(joinInfo)
		ui.Info("加入 Worker 节点...")
	}
	
	ui.SubStep("执行 join 命令...")
	if _, err := nodeClient.Execute(joinCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("加入集群失败: %w", err)
	}
	ui.SubStepDone()
	
	// 如果是 GPU 节点，打标签
	if newNode.GPU {
		ui.SubStep("标记 GPU 节点...")
		if err := LabelGPUNode(masterClient, newNode.Hostname); err != nil {
			ui.SubStepFailed()
			ui.Warning("标记 GPU 节点失败: %v", err)
		} else {
			ui.SubStepDone()
		}
	}
	
	// 验证节点状态
	ui.SubStep("验证节点状态...")
	output, err := masterClient.Execute(fmt.Sprintf("kubectl get node %s", newNode.Hostname))
	if err != nil {
		ui.SubStepFailed()
		ui.Warning("获取节点状态失败: %v", err)
	} else {
		ui.SubStepDone()
		ui.Info("节点状态:\n%s", output)
	}
	
	ui.Success("节点 %s 已成功添加到集群！", newNode.Hostname)
	return nil
}

// RemoveNode 从集群删除节点
func RemoveNode(masterIP string, masterSSHConfig config.SSHConfig, nodeName string, reset bool) error {
	ui.Header(fmt.Sprintf("删除节点: %s", nodeName))
	
	masterClient, err := executor.NewSSHClient(masterIP, masterSSHConfig.Port, masterSSHConfig.User, masterSSHConfig.KeyFile)
	if err != nil {
		return fmt.Errorf("连接 master 节点失败: %w", err)
	}
	defer masterClient.Close()
	
	// 步骤 1: Drain 节点
	ui.Step(1, 3, "驱逐节点上的 Pod")
	ui.SubStep("执行 kubectl drain...")
	
	drainCmd := fmt.Sprintf("kubectl drain %s --delete-emptydir-data --ignore-daemonsets --force --timeout=300s", nodeName)
	if _, err := masterClient.Execute(drainCmd); err != nil {
		ui.SubStepFailed()
		ui.Warning("驱逐 Pod 失败: %v", err)
		// 继续执行
	} else {
		ui.SubStepDone()
	}
	
	// 步骤 2: Delete 节点
	ui.Step(2, 3, "从集群删除节点")
	ui.SubStep("执行 kubectl delete node...")
	
	deleteCmd := fmt.Sprintf("kubectl delete node %s", nodeName)
	if _, err := masterClient.Execute(deleteCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("删除节点失败: %w", err)
	}
	ui.SubStepDone()
	
	// 步骤 3: 可选的 reset 操作
	if reset {
		ui.Step(3, 3, "重置节点（可选）")
		ui.Warning("需要手动在节点上执行: kubeadm reset -f")
		// 如果有节点的 SSH 信息，可以在这里执行 reset
	} else {
		ui.Step(3, 3, "跳过节点重置")
	}
	
	ui.Success("节点 %s 已从集群删除！", nodeName)
	return nil
}

// ListNodes 列出集群的所有节点
func ListNodes(masterIP string, masterSSHConfig config.SSHConfig) error {
	client, err := executor.NewSSHClient(masterIP, masterSSHConfig.Port, masterSSHConfig.User, masterSSHConfig.KeyFile)
	if err != nil {
		return fmt.Errorf("连接 master 节点失败: %w", err)
	}
	defer client.Close()
	
	ui.Info("获取节点列表...")
	output, err := client.Execute("kubectl get nodes -o wide")
	if err != nil {
		return fmt.Errorf("获取节点列表失败: %w", err)
	}
	
	fmt.Println(output)
	return nil
}

// GetNodeInfo 获取节点详细信息
func GetNodeInfo(masterIP string, masterSSHConfig config.SSHConfig, nodeName string) error {
	client, err := executor.NewSSHClient(masterIP, masterSSHConfig.Port, masterSSHConfig.User, masterSSHConfig.KeyFile)
	if err != nil {
		return fmt.Errorf("连接 master 节点失败: %w", err)
	}
	defer client.Close()
	
	ui.Info("获取节点详细信息: %s", nodeName)
	
	// 基本信息
	output, err := client.Execute(fmt.Sprintf("kubectl describe node %s", nodeName))
	if err != nil {
		return fmt.Errorf("获取节点信息失败: %w", err)
	}
	
	fmt.Println(output)
	return nil
}

// CordonNode 标记节点为不可调度
func CordonNode(masterIP string, masterSSHConfig config.SSHConfig, nodeName string) error {
	client, err := executor.NewSSHClient(masterIP, masterSSHConfig.Port, masterSSHConfig.User, masterSSHConfig.KeyFile)
	if err != nil {
		return err
	}
	defer client.Close()
	
	ui.Info("标记节点 %s 为不可调度...", nodeName)
	_, err = client.Execute(fmt.Sprintf("kubectl cordon %s", nodeName))
	if err != nil {
		return fmt.Errorf("cordon 节点失败: %w", err)
	}
	
	ui.Success("节点 %s 已标记为不可调度", nodeName)
	return nil
}

// UncordonNode 取消节点不可调度标记
func UncordonNode(masterIP string, masterSSHConfig config.SSHConfig, nodeName string) error {
	client, err := executor.NewSSHClient(masterIP, masterSSHConfig.Port, masterSSHConfig.User, masterSSHConfig.KeyFile)
	if err != nil {
		return err
	}
	defer client.Close()
	
	ui.Info("取消节点 %s 的不可调度标记...", nodeName)
	_, err = client.Execute(fmt.Sprintf("kubectl uncordon %s", nodeName))
	if err != nil {
		return fmt.Errorf("uncordon 节点失败: %w", err)
	}
	
	ui.Success("节点 %s 已恢复调度", nodeName)
	return nil
}

