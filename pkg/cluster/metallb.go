package cluster

import (
	"fmt"
	"time"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/packages"
	"stormdragon/k8s-deployer/pkg/ui"
)

// InstallMetalLB 安装 MetalLB
func InstallMetalLB(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	ui.Header("安装 MetalLB LoadBalancer")

	// 检查是否需要安装 MetalLB
	if !cfg.Spec.BGP.Enabled && cfg.Spec.LoadBalancer.Mode != "l2" {
		ui.Info("LoadBalancer 未启用，跳过 MetalLB 安装")
		return nil
	}

	// 步骤 1: 部署 MetalLB
	ui.Step(1, 3, "部署 MetalLB")
	if err := deployMetalLBHelm(client, cfg); err != nil {
		return err
	}

	// 步骤 2: 等待 MetalLB 就绪
	ui.Step(2, 3, "等待 MetalLB 就绪")
	if err := waitForMetalLB(client); err != nil {
		return err
	}

	// 步骤 3: 配置 MetalLB（IP Pool 和 BGP/L2）
	ui.Step(3, 3, "配置 MetalLB")
	if cfg.Spec.BGP.Enabled {
		if err := configureMetalLBBGP(client, cfg); err != nil {
			return err
		}
	} else {
		if err := configureMetalLBL2(client, cfg); err != nil {
			return err
		}
	}

	ui.Success("MetalLB 安装完成！")
	return nil
}

// deployMetalLBHelm 使用 Helm 部署 MetalLB
func deployMetalLBHelm(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	ui.SubStep("检查 MetalLB Helm chart...")

	// 初始化包管理器
	pkgMgr := packages.NewManager()

	// 检查本地 MetalLB Helm chart
	chartPath := pkgMgr.GetPackagePath("metallb-chart")
	if !pkgMgr.Exists("metallb-chart") {
		ui.SubStepFailed()
		return fmt.Errorf("缺少 MetalLB Helm chart: %s", chartPath)
	}
	ui.SubStepDone()

	// 解析镜像仓库
	imageRegistry := parseImageRegistry(cfg.Spec.ImageRepository)

	ui.SubStep("安装 MetalLB...")
	installCmd := fmt.Sprintf(`helm install metallb %s `+
		`--namespace metallb-system --create-namespace `+
		`--set controller.image.registry=%s `+
		`--set controller.image.repository=metallb-controller `+
		`--set speaker.image.registry=%s `+
		`--set speaker.image.repository=metallb-speaker `+
		`--wait`,
		chartPath, imageRegistry, imageRegistry)

	if _, err := client.Execute(installCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("安装 MetalLB 失败: %w", err)
	}
	ui.SubStepDone()

	return nil
}

// waitForMetalLB 等待 MetalLB 就绪
func waitForMetalLB(client executor.CommandExecutor) error {
	ui.SubStep("等待 MetalLB Controller 就绪...")

	// 等待 Deployment 就绪（最多 3 分钟）
	cmd := "kubectl rollout status deployment/metallb-controller -n metallb-system --timeout=180s"
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("MetalLB Controller 未能在 3 分钟内就绪: %w", err)
	}
	ui.SubStepDone()

	ui.SubStep("等待 MetalLB Speaker 就绪...")
	cmd = "kubectl rollout status daemonset/metallb-speaker -n metallb-system --timeout=180s"
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("MetalLB Speaker 未能在 3 分钟内就绪: %w", err)
	}
	ui.SubStepDone()

	// 额外等待，确保 CRD 完全就绪
	time.Sleep(5 * time.Second)

	return nil
}

// configureMetalLBBGP 配置 MetalLB BGP 模式
func configureMetalLBBGP(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	// 这个函数将在 bgp.go 中实现
	// 这里只是占位符
	ui.SubStep("配置 MetalLB BGP...")
	if err := ConfigureMetalLBBGP(client, cfg); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()
	return nil
}

// configureMetalLBL2 配置 MetalLB L2 模式
func configureMetalLBL2(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	ui.SubStep("配置 MetalLB L2 模式...")

	// 创建 IP Address Pool
	if err := createMetalLBIPPool(client, cfg); err != nil {
		ui.SubStepFailed()
		return err
	}

	// 创建 L2 Advertisement
	l2AdvYAML := fmt.Sprintf(`apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: %s-l2-adv
  namespace: metallb-system
spec:
  ipAddressPools:
  - %s-ip-pool
`, cfg.Metadata.Name, cfg.Metadata.Name)

	cmd := fmt.Sprintf(`echo '%s' | kubectl apply -f -`, l2AdvYAML)
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("创建 L2Advertisement 失败: %w", err)
	}

	ui.SubStepDone()
	return nil
}

// createMetalLBIPPool 创建 MetalLB IP Address Pool
func createMetalLBIPPool(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	if len(cfg.Spec.BGP.LoadBalancerIPs) == 0 {
		return fmt.Errorf("LoadBalancerIPs 配置为空")
	}

	// 构建 IP 地址列表
	addresses := ""
	for _, ipEntry := range cfg.Spec.BGP.LoadBalancerIPs {
		addresses += fmt.Sprintf("  - %s\n", ipEntry)
	}

	ipPoolYAML := fmt.Sprintf(`apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: %s-ip-pool
  namespace: metallb-system
spec:
  addresses:
%s`, cfg.Metadata.Name, addresses)

	cmd := fmt.Sprintf(`echo '%s' | kubectl apply -f -`, ipPoolYAML)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("创建 IPAddressPool 失败: %w", err)
	}

	return nil
}

// UninstallMetalLB 卸载 MetalLB
func UninstallMetalLB(client executor.CommandExecutor) error {
	ui.Info("卸载 MetalLB...")

	// 删除 MetalLB 配置资源
	cmds := []string{
		"kubectl delete -n metallb-system ipaddresspool --all",
		"kubectl delete -n metallb-system bgppeer --all",
		"kubectl delete -n metallb-system bgpadvertisement --all",
		"kubectl delete -n metallb-system l2advertisement --all",
	}

	for _, cmd := range cmds {
		client.Execute(cmd) // 忽略错误，继续删除
	}

	// 卸载 Helm release
	if _, err := client.Execute("helm uninstall metallb -n metallb-system"); err != nil {
		ui.Warning("卸载 MetalLB Helm release 失败: %v", err)
	}

	// 删除 namespace
	client.Execute("kubectl delete namespace metallb-system")

	return nil
}
