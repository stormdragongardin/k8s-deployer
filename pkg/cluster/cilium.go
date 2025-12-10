package cluster

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"time"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/packages"
	"stormdragon/k8s-deployer/pkg/ui"
)

//go:embed templates/cilium-values.yaml
var ciliumValuesTemplate string

//go:embed templates/default-gateway.yaml
var defaultGatewayTemplate string

// CiliumValuesConfig Cilium values 模板参数
type CiliumValuesConfig struct {
	ImageRegistry        string
	K8sServiceHost       string
	K8sServicePort       string
	PodSubnet            string
	HubbleEnabled        bool
	HubbleUIEnabled      bool
	HubbleUINodePort     int
	HubbleMetricsEnabled bool
	BGPEnabled           bool
	LoadBalancerMode     string
	GatewayAPIEnabled    bool
	EnvoyEnabled         bool
}

// InstallCilium 安装 Cilium 网络插件（离线）
func InstallCilium(client *executor.SSHClient, cfg *config.ClusterConfig, controlPlaneEndpoint string) error {
	ui.Header("安装 Cilium 网络插件")

	// 步骤 1: 安装 Helm（离线）
	ui.Step(1, 4, "安装 Helm")
	if err := installHelmOffline(client); err != nil {
		return err
	}

	// 步骤 2: 安装 Cilium（离线）
	ui.Step(2, 4, "部署 Cilium")
	if err := deployCiliumOffline(client, cfg, controlPlaneEndpoint); err != nil {
		return err
	}

	// 步骤 3: 验证 Cilium
	ui.Step(3, 4, "验证 Cilium 状态")
	if err := verifyCilium(client); err != nil {
		return err
	}

	// 步骤 4: 部署默认 Gateway（如果启用了 Gateway API）
	if cfg.Spec.GatewayAPI.Enabled {
		ui.Step(4, 4, "部署默认 Gateway")
		if err := deployDefaultGateway(client, cfg); err != nil {
			ui.Warning("部署默认 Gateway 失败: %v", err)
			ui.Info("  您可以稍后手动部署: kubectl apply -f examples/default-gateway.yaml")
		}
	}

	ui.Success("Cilium 安装完成！")
	ui.Info("  网络插件: Cilium v1.18.4")
	ui.Info("  模式: kube-proxy replacement (eBPF)")
	if cfg.Spec.Hubble.Enabled {
		ui.Info("  Hubble: 已启用")
		if cfg.Spec.Hubble.UI.Enabled && cfg.Spec.Hubble.UI.NodePort > 0 {
			ui.Info("  Hubble UI: http://<节点IP>:%d", cfg.Spec.Hubble.UI.NodePort)
		}
	}
	if cfg.Spec.GatewayAPI.Enabled && cfg.Spec.BGP.Enabled {
		ui.Info("  Gateway API: 已启用")
		ui.Info("  默认 Gateway: default-gateway (http://10.0.6.1)")
	}

	return nil
}

// installHelmOffline 离线安装 Helm
func installHelmOffline(client *executor.SSHClient) error {
	// 初始化包管理器
	pkgMgr := packages.NewManager()

	// 检查本地离线包
	ui.SubStep("检查 Helm 离线包...")
	helmPath := pkgMgr.GetPackagePath("helm")
	if !pkgMgr.Exists("helm") {
		ui.SubStepFailed()
		return fmt.Errorf("缺少离线包: %s，请先运行: cd scripts && ./download-all.sh", helmPath)
	}
	ui.SubStepDone()

	// 检查是否已安装（用于提示）
	ui.SubStep("安装 Helm...")
	if _, err := client.Execute("which helm"); err == nil {
		ui.Info("  覆盖现有 Helm...")
	}

	// 上传 Helm 二进制文件（覆盖）
	remotePath := "/usr/local/bin/helm"
	if err := client.UploadFile(helmPath, remotePath); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 Helm 失败: %w", err)
	}

	// 设置执行权限
	if _, err := client.Execute(fmt.Sprintf("chmod +x %s", remotePath)); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("设置 Helm 权限失败: %w", err)
	}
	ui.SubStepDone()

	return nil
}

// deployCiliumOffline 离线部署 Cilium
func deployCiliumOffline(client *executor.SSHClient, cfg *config.ClusterConfig, controlPlaneEndpoint string) error {
	ui.SubStep("检查 Cilium Chart 离线包...")

	// 初始化包管理器
	pkgMgr := packages.NewManager()

	// 检查本地 Cilium chart
	chartPath := pkgMgr.GetPackagePath("cilium-chart")
	if !pkgMgr.Exists("cilium-chart") {
		ui.SubStepFailed()
		return fmt.Errorf("缺少 Cilium Chart 离线包: %s，请先运行: cd scripts && ./download-all.sh", chartPath)
	}
	ui.SubStepDone()

	// 上传 Cilium chart
	ui.SubStep("上传 Cilium Chart...")
	remoteChartPath := "/tmp/cilium.tgz"
	if err := client.UploadFile(chartPath, remoteChartPath); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 Cilium Chart 失败: %w", err)
	}
	ui.SubStepDone()

	// 解析镜像仓库地址（移除协议和路径）
	registry := parseImageRegistry(cfg.Spec.ImageRepository)

	// 生成 Cilium values 文件
	ui.SubStep("生成 Cilium 配置...")
	valuesContent, err := generateCiliumValues(cfg, controlPlaneEndpoint, registry)
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("生成 Cilium 配置失败: %w", err)
	}

	// 上传 values 文件
	remoteValuesPath := "/tmp/cilium-values.yaml"
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", remoteValuesPath, valuesContent)
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 Cilium 配置失败: %w", err)
	}
	ui.SubStepDone()
	ui.Info("  使用镜像仓库: %s", registry)
	if cfg.Spec.BGP.Enabled {
		ui.Info("  BGP 模式: 已启用")
	}

	// 安装 Cilium
	ui.SubStep("安装 Cilium (kube-proxy 替代模式)...")

	// 构建 Helm 安装命令（使用本地 chart 和 values 文件）
	installCmd := fmt.Sprintf(`helm install cilium %s \
		--namespace kube-system \
		--values %s`,
		remoteChartPath, remoteValuesPath)

	if _, err := client.Execute(installCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("部署 Cilium 失败: %w", err)
	}
	ui.SubStepDone()

	// 清理临时文件
	client.Execute(fmt.Sprintf("rm -f %s %s", remoteChartPath, remoteValuesPath))

	return nil
}

// generateCiliumValues 生成 Cilium values 配置
func generateCiliumValues(cfg *config.ClusterConfig, controlPlaneEndpoint, imageRegistry string) (string, error) {
	// 默认 LoadBalancer 模式为 DSR
	lbMode := "dsr"
	if cfg.Spec.LoadBalancer.Mode != "" {
		lbMode = cfg.Spec.LoadBalancer.Mode
	}

	params := CiliumValuesConfig{
		ImageRegistry:        imageRegistry,
		K8sServiceHost:       controlPlaneEndpoint,
		K8sServicePort:       "6443",
		PodSubnet:            cfg.Spec.Networking.PodSubnet,
		HubbleEnabled:        cfg.Spec.Hubble.Enabled,
		HubbleUIEnabled:      cfg.Spec.Hubble.UI.Enabled,
		HubbleUINodePort:     cfg.Spec.Hubble.UI.NodePort,
		HubbleMetricsEnabled: cfg.Spec.Hubble.Metrics.Enabled,
		BGPEnabled:           cfg.Spec.BGP.Enabled,
		LoadBalancerMode:     lbMode,
		GatewayAPIEnabled:    cfg.Spec.GatewayAPI.Enabled,
		EnvoyEnabled:         cfg.Spec.Envoy.Enabled,
	}

	tmpl, err := template.New("cilium-values").Parse(ciliumValuesTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// parseImageRegistry 解析镜像仓库地址
func parseImageRegistry(imageRepo string) string {
	// 移除协议前缀
	if len(imageRepo) > 7 && imageRepo[:7] == "http://" {
		imageRepo = imageRepo[7:]
	} else if len(imageRepo) > 8 && imageRepo[:8] == "https://" {
		imageRepo = imageRepo[8:]
	}
	return imageRepo
}

// verifyCilium 验证 Cilium 状态
func verifyCilium(client *executor.SSHClient) error {
	ui.SubStep("等待 Cilium DaemonSet 就绪...")

	// 等待 Cilium DaemonSet 就绪（最多 5 分钟）
	maxRetries := 60
	for i := 0; i < maxRetries; i++ {
		output, err := client.Execute("kubectl get ds cilium -n kube-system -o jsonpath='{.status.numberReady}/{.status.desiredNumberScheduled}'")
		if err == nil && output != "" {
			// 检查是否所有副本都就绪
			if output[0] != '0' && len(output) > 2 {
				// 简单检查，如果有输出且不是 0/x 格式
				ui.SubStepDone()
				ui.Info("Cilium DaemonSet 状态: %s", output)
				break
			}
		}

		if i == maxRetries-1 {
			ui.SubStepFailed()
			return fmt.Errorf("cilium DaemonSet 未能在 5 分钟内就绪")
		}

		time.Sleep(5 * time.Second)
	}

	// 验证 kube-proxy 不存在
	ui.SubStep("确认 kube-proxy 已移除...")
	_, err := client.Execute("kubectl get ds kube-proxy -n kube-system")
	if err == nil {
		ui.SubStepFailed()
		ui.Warning("检测到 kube-proxy 仍然存在，Cilium 可能未正确替代")
	} else {
		ui.SubStepDone()
		ui.Success("kube-proxy 已被 Cilium 替代")
	}

	// 检查 Cilium 状态
	ui.SubStep("检查 Cilium 运行状态...")
	output, err := client.Execute("kubectl get pods -n kube-system -l k8s-app=cilium")
	if err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("获取 Cilium Pods 状态失败: %w", err)
	}
	ui.SubStepDone()
	ui.Info("Cilium Pods:\n%s", output)

	return nil
}

// deployDefaultGateway 部署默认 Gateway 资源
func deployDefaultGateway(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	ui.SubStep("等待 GatewayClass 就绪...")
	
	// 等待 Cilium GatewayClass 创建（最多 1 分钟）
	maxRetries := 12
	for i := 0; i < maxRetries; i++ {
		output, err := client.Execute("kubectl get gatewayclass cilium -o jsonpath='{.status.conditions[?(@.type==\"Accepted\")].status}'")
		if err == nil && output == "True" {
			ui.SubStepDone()
			break
		}
		
		if i == maxRetries-1 {
			ui.SubStepFailed()
			return fmt.Errorf("GatewayClass cilium 未能在 1 分钟内就绪")
		}
		
		time.Sleep(5 * time.Second)
	}
	
	ui.SubStep("部署默认 Gateway 资源...")
	
	// 上传 Gateway YAML
	remoteGatewayPath := "/tmp/default-gateway.yaml"
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", remoteGatewayPath, defaultGatewayTemplate)
	if _, err := client.Execute(cmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("创建 Gateway 配置失败: %w", err)
	}
	
	// 应用 Gateway
	if _, err := client.Execute(fmt.Sprintf("kubectl apply -f %s", remoteGatewayPath)); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("部署 Gateway 失败: %w", err)
	}
	
	// 清理临时文件
	client.Execute(fmt.Sprintf("rm -f %s", remoteGatewayPath))
	ui.SubStepDone()
	
	// 等待 Gateway 就绪
	ui.SubStep("等待 Gateway 获取 LoadBalancer IP...")
	for i := 0; i < 12; i++ {
		output, err := client.Execute("kubectl get gateway default-gateway -n default -o jsonpath='{.status.addresses[0].value}'")
		if err == nil && output != "" {
			ui.SubStepDone()
			ui.Success("默认 Gateway 部署完成！")
			ui.Info("  Gateway: default-gateway")
			ui.Info("  地址: %s", output)
			ui.Info("  端口: HTTP(80), HTTPS(443)")
			return nil
		}
		
		if i == 11 {
			ui.SubStepFailed()
			ui.Warning("Gateway 未能在 1 分钟内获取 IP，请稍后检查")
			return nil // 不返回错误，让部署继续
		}
		
		time.Sleep(5 * time.Second)
	}
	
	return nil
}

// UninstallCilium 卸载 Cilium
func UninstallCilium(client *executor.SSHClient) error {
	ui.Info("卸载 Cilium...")
	_, err := client.Execute("helm uninstall cilium -n kube-system")
	if err != nil {
		return fmt.Errorf("卸载 Cilium 失败: %w", err)
	}
	return nil
}
