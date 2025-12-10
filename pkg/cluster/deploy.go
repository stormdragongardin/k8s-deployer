package cluster

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/kubeadm"
	"stormdragon/k8s-deployer/pkg/packages"
	"stormdragon/k8s-deployer/pkg/ui"
)

// DeployCluster 部署集群
func DeployCluster(cfg *config.ClusterConfig, autoConfirm bool) error {
	ui.Header(fmt.Sprintf("部署集群: %s (v%s)", cfg.Metadata.Name, cfg.Spec.Version))
	
	// 显示集群信息
	masterCount := 0
	workerCount := 0
	gpuCount := 0
	for _, node := range cfg.Spec.Nodes {
		switch node.Role {
		case "master":
			masterCount++
		case "worker":
			workerCount++
			if node.GPU {
				gpuCount++
			}
		}
	}
	ui.PrintClusterInfo(cfg.Metadata.Name, cfg.Spec.Version, masterCount, workerCount, gpuCount)
	
	// 确认部署
	if !autoConfirm && !ui.WaitForConfirmation("确认开始部署？") {
		ui.Warning("部署已取消")
		return nil
	}
	
	// ========================================
	// 阶段 1: 基础环境检查和准备
	// ========================================
	ui.Header("阶段 1: 基础环境检查和准备")
	
	// 1.1 检查 SSH 连接
	ui.Step(1, 4, "检查 SSH 连接")
	if err := checkSSHConnections(cfg); err != nil {
		return err
	}
	
	// 1.2 系统优化和节点准备
	ui.Step(2, 4, "系统优化和节点准备")
	if err := prepareAllNodes(cfg); err != nil {
		return err
	}
	
	// 1.3 配置负载均衡器（如果是 HA）
	var firstMasterIP string
	if cfg.Spec.HA.Enabled {
		ui.Step(3, 4, "配置高可用负载均衡器")
		firstMasterIP = getFirstMasterIP(cfg)
		if err := setupHAProxy(cfg, firstMasterIP); err != nil {
			return err
		}
	} else {
		firstMasterIP = getFirstMasterIP(cfg)
		ui.Step(3, 4, "跳过负载均衡器配置（非 HA 模式）")
	}
	
	// ========================================
	// 阶段 2: 部署 Master 节点和创建集群
	// ========================================
	ui.Header("阶段 2: 部署 Master 节点和创建集群")
	
	// 2.1 初始化第一个 Master
	ui.Step(1, 3, "初始化第一个 Master 节点")
	joinInfo, err := initFirstMaster(cfg, firstMasterIP)
	if err != nil {
		return err
	}
	
	// 2.2 加入其他 Master 节点（如果有）
	otherMasters := getOtherMasters(cfg, firstMasterIP)
	if len(otherMasters) > 0 {
		ui.Step(2, 3, "加入其他 %d 个 Master 节点", len(otherMasters))
		if err := joinMasters(otherMasters, joinInfo); err != nil {
			return err
		}
	}
	
	// ========================================
	// 阶段 2.5: 配置本地 kubectl
	// ========================================
	ui.Header("配置本地 kubectl")
	client, _ := executor.NewSSHClient(firstMasterIP, 22, "root", cfg.Spec.Nodes[0].SSH.KeyFile)
	defer client.Close()
	
	if err := setupLocalKubectl(client, cfg); err != nil {
		ui.Warning("配置本地 kubectl 失败: %v", err)
		ui.Info("您可以手动获取 kubeconfig：")
		ui.Info("  scp root@%s:/etc/kubernetes/admin.conf ~/.kube/config", firstMasterIP)
	} else {
		ui.Success("本地 kubectl 配置完成！")
	}
	
	// ========================================
	// 阶段 3: 安装 Cilium（替代 kube-proxy）
	// ========================================
	ui.Header("阶段 3: 安装 Cilium 网络插件")
	
	controlPlaneEndpoint := firstMasterIP
	if cfg.Spec.HA.Enabled {
		controlPlaneEndpoint = cfg.Spec.HA.VIP
	}
	
	if err := InstallCilium(client, cfg, controlPlaneEndpoint); err != nil {
		return err
	}

	// ========================================
	// 阶段 3.5: 安装 MetalLB LoadBalancer（如果启用）
	// ========================================
	if cfg.Spec.LoadBalancer.Provider == "metallb" || cfg.Spec.BGP.Enabled {
		ui.Header("阶段 3.5: 安装 MetalLB LoadBalancer")
		
		// 使用本地 kubectl 执行器
		localClient := executor.NewLocalExecutor()
		if err := InstallMetalLB(localClient, cfg); err != nil {
			return fmt.Errorf("安装 MetalLB 失败: %w", err)
		}
	}

	// ========================================
	// 阶段 4: 加入 Worker 节点
	// ========================================
	ui.Header("阶段 4: 加入 Worker 节点")
	
	workers := getWorkers(cfg)
	if len(workers) > 0 {
		ui.Step(1, 1, "加入 %d 个 Worker 节点", len(workers))
		if err := joinWorkers(workers, joinInfo); err != nil {
			return err
		}
	}

	// ========================================
	// 阶段 5: GPU 节点配置
	// ========================================
	gpuNodes := getGPUNodes(cfg)
	if len(gpuNodes) > 0 {
		ui.Header("阶段 5: 配置 GPU 节点")
		ui.Step(1, 1, "标记 %d 个 GPU 节点", len(gpuNodes))
		
		for _, node := range gpuNodes {
			if err := LabelGPUNode(client, node.Hostname); err != nil {
				ui.Warning("标记 GPU 节点 %s 失败: %v", node.Hostname, err)
			}
		}
	}

	// ========================================
	// 阶段 6: 验证集群
	// ========================================
	ui.Header("阶段 6: 集群验证")
	if err := validateCluster(client); err != nil {
		return err
	}

	// ========================================
	// 阶段 7: 保存集群配置
	// ========================================
	if err := SaveClusterConfig(client, cfg); err != nil {
		ui.Warning("保存集群配置失败: %v", err)
		ui.Warning("这不影响集群使用，但可能影响后续的 update 命令")
	}

	// 显示完成信息
	ui.Header("✓ 集群部署完成！")
	printClusterSummary(cfg, firstMasterIP)
	
	return nil
}

// checkSSHConnections 检查所有节点的 SSH 连接（并发）
func checkSSHConnections(cfg *config.ClusterConfig) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Spec.Nodes))
	
	for i := range cfg.Spec.Nodes {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			node := cfg.Spec.Nodes[idx]
			
			ui.SubStep("[%d/%d] 检查节点 %s (%s)...", idx+1, len(cfg.Spec.Nodes), node.Hostname, node.IP)
			
			if err := executor.TestConnection(node.IP, node.SSH.Port, node.SSH.User, node.SSH.KeyFile); err != nil {
				ui.SubStepFailed()
				errChan <- fmt.Errorf("节点 %s SSH 连接失败: %w", node.IP, err)
				return
			}
			ui.SubStepDone()
		}(i)
	}
	
	wg.Wait()
	close(errChan)
	
	// 检查是否有错误
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// prepareAllNodes 准备所有节点（并发，带颜色日志）
func prepareAllNodes(cfg *config.ClusterConfig) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.Spec.Nodes))
	
	// 创建节点名称列表
	nodeNames := make([]string, len(cfg.Spec.Nodes))
	for i, node := range cfg.Spec.Nodes {
		nodeNames[i] = node.Hostname
	}
	
	// 创建并发日志器
	logger := ui.NewSimpleProgressLogger(nodeNames)
	
	ui.Info("并发准备 %d 个节点...", len(cfg.Spec.Nodes))
	ui.Info("")
	
	for i := range cfg.Spec.Nodes {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			node := &cfg.Spec.Nodes[idx]
			
			logger.Log(node.Hostname, "系统优化中...")
			
			// 使用静默版本，避免输出混乱
			if err := PrepareNodeQuiet(node, cfg.Spec.ImageRepository, cfg.Spec.Version); err != nil {
				logger.Error(node.Hostname, fmt.Sprintf("准备失败: %v", err))
				errChan <- fmt.Errorf("准备节点 %s 失败: %w", node.Hostname, err)
				return
			}
			
			logger.Success(node.Hostname, "节点准备完成")
		}(i)
	}
	
	wg.Wait()
	close(errChan)
	
	// 检查是否有错误
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	ui.Info("")
	ui.Success("所有节点准备完成！")
	return nil
}

// setupHAProxy 配置 HAProxy 负载均衡器
func setupHAProxy(cfg *config.ClusterConfig, firstMasterIP string) error {
	client, err := executor.NewSSHClient(firstMasterIP, 22, "root", cfg.Spec.Nodes[0].SSH.KeyFile)
	if err != nil {
		return err
	}
	defer client.Close()
	
	ui.SubStep("安装 HAProxy...")
	
	installScript := `
		# 检测操作系统
		if [ -f /etc/os-release ]; then
			. /etc/os-release
			OS=$ID
		fi
		
		# 安装 HAProxy
		if [ "$OS" = "ubuntu" ] || [ "$OS" = "debian" ]; then
			apt-get update
			apt-get install -y haproxy
		elif [ "$OS" = "centos" ] || [ "$OS" = "rhel" ]; then
			yum install -y haproxy
		fi
	`
	
	if _, err := client.Execute(installScript); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.SubStep("配置 HAProxy...")
	
	// 生成 HAProxy 配置
	var backends strings.Builder
	for i, node := range cfg.Spec.Nodes {
		if node.Role == "master" {
			backends.WriteString(fmt.Sprintf("    server master-%d %s:6443 check\n", i+1, node.IP))
		}
	}
	
	haproxyConfig := fmt.Sprintf(`
global
    log /dev/log local0
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

defaults
    log     global
    mode    tcp
    option  tcplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend k8s-api
    bind *:6443
    mode tcp
    default_backend k8s-api-backend

backend k8s-api-backend
    mode tcp
    balance roundrobin
%s
`, backends.String())
	
	// 写入配置
	tmpFile := "/tmp/haproxy.cfg"
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", tmpFile, haproxyConfig)
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return err
	}
	
	_, err = client.Execute("mv /tmp/haproxy.cfg /etc/haproxy/haproxy.cfg && systemctl restart haproxy && systemctl enable haproxy")
	if err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	
	ui.Success("HAProxy 配置完成，VIP: %s:6443", cfg.Spec.HA.VIP)
	return nil
}

// initFirstMaster 初始化第一个 Master 节点
func initFirstMaster(cfg *config.ClusterConfig, masterIP string) (*kubeadm.JoinCommand, error) {
	client, err := executor.NewSSHClient(masterIP, 22, "root", cfg.Spec.Nodes[0].SSH.KeyFile)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	
	// 检查是否已经初始化
	ui.SubStep("检查 Master 节点状态...")
	if _, err := client.Execute("test -f /etc/kubernetes/admin.conf"); err == nil {
		ui.SubStepDone()
		ui.Warning("检测到 Master 节点已初始化")
		ui.Warning("继续将会重置节点并重新初始化集群")
		ui.Warning("这将导致当前集群不可用！")
		fmt.Println()
		
	if !ui.WaitForDangerousConfirmation("确认重置并重新初始化？") {
		return nil, fmt.Errorf("用户取消操作")
	}
	
	ui.SubStep("彻底重置 Master 节点...")
	
	// 增强的重置命令
	resetCmd := `
		# 停止所有 K8s 组件
		systemctl stop kubelet || true
		
		# 执行 kubeadm reset
		kubeadm reset -f --cri-socket unix:///run/containerd/containerd.sock
		
		# 清理残留进程
		pkill -9 kube-apiserver || true
		pkill -9 kube-controller || true
		pkill -9 kube-scheduler || true
		pkill -9 etcd || true
		
		# 清理残留文件
		rm -rf /etc/kubernetes/*
		rm -rf /var/lib/etcd/*
		rm -rf /var/lib/kubelet/*
		
		# 清理网络配置
		ip link delete cni0 2>/dev/null || true
		ip link delete flannel.1 2>/dev/null || true
		
		# 重启 containerd
		systemctl restart containerd
		
		# 等待 containerd 完全启动
		sleep 3
	`
	
	if _, err := client.Execute(resetCmd); err != nil {
		ui.SubStepFailed()
		return nil, fmt.Errorf("重置节点失败: %w", err)
	}
	ui.SubStepDone()
	} else {
		ui.SubStepDone()
		ui.Info("  Master 节点未初始化，开始部署")
	}
	
	ui.SubStep("生成 kubeadm 配置...")
	
	// 生成 kubeadm 配置
	kubeadmConfig, err := kubeadm.GenerateInitConfig(cfg, masterIP)
	if err != nil {
		ui.SubStepFailed()
		return nil, err
	}
	
	// 上传配置
	tmpFile := "/tmp/kubeadm-init.yaml"
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", tmpFile, kubeadmConfig)
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return nil, err
	}
	ui.SubStepDone()
	
	ui.SubStep("执行 kubeadm init（跳过 kube-proxy）...")
	
	// 执行 kubeadm init，跳过 kube-proxy
	initCmd := kubeadm.GetInitCommand(tmpFile, []string{"addon/kube-proxy"})
	if _, err := client.Execute(initCmd); err != nil {
		ui.SubStepFailed()
		return nil, fmt.Errorf("kubeadm init 失败: %w", err)
	}
	ui.SubStepDone()
	
	ui.SubStep("配置 kubectl...")
	_, err = client.Execute(`
		mkdir -p $HOME/.kube
		cp /etc/kubernetes/admin.conf $HOME/.kube/config
		chown $(id -u):$(id -g) $HOME/.kube/config
	`)
	if err != nil {
		ui.SubStepFailed()
		return nil, err
	}
	ui.SubStepDone()
	
	ui.SubStep("获取 join 信息...")
	
	// 获取 join 信息
	controlPlaneEndpoint := masterIP + ":6443"
	if cfg.Spec.HA.Enabled {
		controlPlaneEndpoint = cfg.Spec.HA.VIP + ":6443"
	}
	
	joinInfo, err := kubeadm.GetJoinInfo(client, controlPlaneEndpoint, true)
	if err != nil {
		ui.SubStepFailed()
		return nil, err
	}
	ui.SubStepDone()
	
	ui.Success("第一个 Master 节点初始化完成！")
	return joinInfo, nil
}

// 辅助函数
func getFirstMasterIP(cfg *config.ClusterConfig) string {
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "master" {
			return node.IP
		}
	}
	return ""
}

func getOtherMasters(cfg *config.ClusterConfig, firstMasterIP string) []config.NodeConfig {
	var masters []config.NodeConfig
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "master" && node.IP != firstMasterIP {
			masters = append(masters, node)
		}
	}
	return masters
}

func getWorkers(cfg *config.ClusterConfig) []config.NodeConfig {
	var workers []config.NodeConfig
	for _, node := range cfg.Spec.Nodes {
		if node.Role == "worker" {
			workers = append(workers, node)
		}
	}
	return workers
}

func getGPUNodes(cfg *config.ClusterConfig) []config.NodeConfig {
	var gpuNodes []config.NodeConfig
	for _, node := range cfg.Spec.Nodes {
		if node.GPU {
			gpuNodes = append(gpuNodes, node)
		}
	}
	return gpuNodes
}

func joinMasters(masters []config.NodeConfig, joinInfo *kubeadm.JoinCommand) error {
	for i, node := range masters {
		ui.SubStep("[%d/%d] 加入 Master: %s...", i+1, len(masters), node.Hostname)
		
		client, err := executor.NewSSHClient(node.IP, node.SSH.Port, node.SSH.User, node.SSH.KeyFile)
		if err != nil {
			ui.SubStepFailed()
			return err
		}
		
		joinCmd := kubeadm.GenerateMasterJoinCommand(joinInfo)
		if _, err := client.Execute(joinCmd); err != nil {
			client.Close()
			ui.SubStepFailed()
			return fmt.Errorf("节点 %s 加入失败: %w", node.Hostname, err)
		}
		
		client.Close()
		ui.SubStepDone()
	}
	return nil
}

func joinWorkers(workers []config.NodeConfig, joinInfo *kubeadm.JoinCommand) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(workers))
	
	// 创建节点名称列表
	workerNames := make([]string, len(workers))
	for i, worker := range workers {
		workerNames[i] = worker.Hostname
	}
	
	// 创建并发日志器
	logger := ui.NewSimpleProgressLogger(workerNames)
	
	ui.Info("并发加入 %d 个 Worker 节点...", len(workers))
	ui.Info("")
	
	for i := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			node := workers[idx]
			
			logger.Log(node.Hostname, "连接节点...")
			
			client, err := executor.NewSSHClient(node.IP, node.SSH.Port, node.SSH.User, node.SSH.KeyFile)
			if err != nil {
				logger.Error(node.Hostname, fmt.Sprintf("连接失败: %v", err))
				errChan <- fmt.Errorf("连接节点 %s 失败: %w", node.Hostname, err)
				return
			}
			defer client.Close()
			
			// 检查节点是否已加入集群
			logger.Log(node.Hostname, "检查节点状态...")
			if _, err := client.Execute("test -f /etc/kubernetes/kubelet.conf"); err == nil {
				// 节点已加入，需要先重置
				logger.Log(node.Hostname, "节点已加入集群，执行重置...")
				resetCmd := "kubeadm reset -f --cri-socket unix:///run/containerd/containerd.sock"
				if _, err := client.Execute(resetCmd); err != nil {
					logger.Error(node.Hostname, fmt.Sprintf("重置失败: %v", err))
					errChan <- fmt.Errorf("节点 %s 重置失败: %w", node.Hostname, err)
					return
				}
			}
			
			logger.Log(node.Hostname, "执行 join 命令...")
			
			joinCmd := kubeadm.GenerateWorkerJoinCommand(joinInfo)
			if _, err := client.Execute(joinCmd); err != nil {
				logger.Error(node.Hostname, fmt.Sprintf("加入失败: %v", err))
				errChan <- fmt.Errorf("节点 %s 加入失败: %w", node.Hostname, err)
				return
			}
			
			logger.Success(node.Hostname, "成功加入集群")
		}(i)
	}
	
	wg.Wait()
	close(errChan)
	
	// 检查是否有错误
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	
	ui.Info("")
	ui.Success("所有 Worker 节点加入完成！")
	return nil
}

func validateCluster(client *executor.SSHClient) error {
	ui.SubStep("检查节点状态...")
	output, err := client.Execute("kubectl get nodes")
	if err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	ui.Info("节点状态:\n%s", output)
	
	ui.SubStep("检查核心组件...")
	output, err = client.Execute("kubectl get pods -n kube-system")
	if err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	ui.Info("核心组件状态:\n%s", output)
	
	return nil
}

func printClusterSummary(cfg *config.ClusterConfig, masterIP string) {
	apiEndpoint := masterIP + ":6443"
	if cfg.Spec.HA.Enabled {
		apiEndpoint = cfg.Spec.HA.VIP + ":6443"
	}
	
	fmt.Printf("\n")
	fmt.Printf("集群信息:\n")
	fmt.Printf("  名称: %s\n", cfg.Metadata.Name)
	fmt.Printf("  版本: %s\n", cfg.Spec.Version)
	fmt.Printf("  API 地址: https://%s\n", apiEndpoint)
	fmt.Printf("  CNI: Cilium (kube-proxy replacement)\n")
	fmt.Printf("  容器运行时: containerd\n")
	fmt.Printf("\n")
	fmt.Printf("获取 kubeconfig:\n")
	fmt.Printf("  $ k8s-deployer cluster kubeconfig %s > ~/.kube/config\n", cfg.Metadata.Name)
	fmt.Printf("\n")
	fmt.Printf("验证集群:\n")
	fmt.Printf("  $ kubectl get nodes\n")
	fmt.Printf("  $ kubectl -n kube-system get pods | grep cilium\n")
	fmt.Printf("\n")
}

// setupLocalKubectl 配置本地 kubectl 和 kubeconfig
func setupLocalKubectl(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	ui.Step(1, 3, "检查本地 kubectl")
	
	// 检查本地是否已安装 kubectl
	_, err := exec.Command("which", "kubectl").Output()
	kubectlExists := (err == nil)
	
	if !kubectlExists {
		ui.SubStep("安装 kubectl...")
		
		// 使用包管理器中的 kubectl
		pkgMgr := packages.NewManagerWithVersion(cfg.Spec.Version)
		kubectlPath := pkgMgr.GetPackagePath("kubectl")
		
		if !pkgMgr.Exists("kubectl") {
			ui.SubStepFailed()
			return fmt.Errorf("本地缺少 kubectl 二进制文件: %s", kubectlPath)
		}
		
		// 复制到 /usr/local/bin
		copyCmd := exec.Command("sudo", "cp", kubectlPath, "/usr/local/bin/kubectl")
		if err := copyCmd.Run(); err != nil {
			ui.SubStepFailed()
			return fmt.Errorf("安装 kubectl 失败: %w", err)
		}
		
		// 设置执行权限
		chmodCmd := exec.Command("sudo", "chmod", "+x", "/usr/local/bin/kubectl")
		if err := chmodCmd.Run(); err != nil {
			ui.SubStepFailed()
			return fmt.Errorf("设置 kubectl 权限失败: %w", err)
		}
		
		ui.SubStepDone()
	} else {
		ui.Info("  kubectl 已安装")
	}
	
	ui.Step(2, 3, "获取 kubeconfig")
	
	// 从 Master 节点获取 admin.conf
	ui.SubStep("下载 kubeconfig...")
	kubeconfigContent, err := client.Execute("cat /etc/kubernetes/admin.conf")
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("读取 kubeconfig 失败: %w", err)
	}
	ui.SubStepDone()
	
	ui.Step(3, 3, "配置 kubeconfig")
	
	// 创建 .kube 目录
	ui.SubStep("保存 kubeconfig...")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("获取 home 目录失败: %w", err)
	}
	
	kubeDir := filepath.Join(homeDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("创建 .kube 目录失败: %w", err)
	}
	
	kubeconfigPath := filepath.Join(kubeDir, "config")
	
	// 备份现有 kubeconfig（如果存在）
	if _, err := os.Stat(kubeconfigPath); err == nil {
		backupPath := kubeconfigPath + ".backup." + cfg.Metadata.Name
		if err := os.Rename(kubeconfigPath, backupPath); err != nil {
			ui.Warning("备份现有 kubeconfig 失败: %v", err)
		} else {
			ui.Info("  现有 kubeconfig 已备份: %s", backupPath)
		}
	}
	
	// 写入新的 kubeconfig
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0600); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("写入 kubeconfig 失败: %w", err)
	}
	
	ui.SubStepDone()
	ui.Info("  kubeconfig 已保存到: %s", kubeconfigPath)
	
	return nil
}
