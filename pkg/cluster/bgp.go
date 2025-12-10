package cluster

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

//go:embed templates/metallb-bgp.yaml
var metallbBGPTemplate string

// MetalLBBGPConfig MetalLB BGP 配置参数
type MetalLBBGPConfig struct {
	ClusterName     string
	LocalASN        int
	BGPPeers        []config.BGPPeerConfig
	LoadBalancerIPs []string
}

// ConfigureMetalLBBGP 配置 MetalLB BGP 模式
func ConfigureMetalLBBGP(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	ui.SubStep("创建 IP Address Pool...")
	if err := createMetalLBIPPool(client, cfg); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()

	ui.SubStep("创建 BGP Peers...")
	if err := createMetalLBBGPPeers(client, cfg); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()

	ui.SubStep("创建 BGP Advertisement...")
	if err := createMetalLBBGPAdvertisement(client, cfg); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()

	ui.SubStep("验证 MetalLB BGP 配置...")
	if err := verifyMetalLBBGP(client); err != nil {
		ui.SubStepFailed()
		return err
	}
	ui.SubStepDone()

	return nil
}

// createMetalLBBGPPeers 创建 MetalLB BGP Peers
func createMetalLBBGPPeers(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	for i, peer := range cfg.Spec.BGP.Peers {
		peerYAML := fmt.Sprintf(`apiVersion: metallb.io/v1beta2
kind: BGPPeer
metadata:
  name: %s-peer-%d
  namespace: metallb-system
spec:
  myASN: %d
  peerASN: %d
  peerAddress: %s
`, cfg.Metadata.Name, i, cfg.Spec.BGP.LocalASN, peer.PeerASN, peer.PeerAddress)

		cmd := fmt.Sprintf(`echo '%s' | kubectl apply -f -`, peerYAML)
		if _, err := client.Execute(cmd); err != nil {
			return fmt.Errorf("创建 BGPPeer %d 失败: %w", i, err)
		}
	}

	return nil
}

// createMetalLBBGPAdvertisement 创建 MetalLB BGP Advertisement
func createMetalLBBGPAdvertisement(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	advYAML := fmt.Sprintf(`apiVersion: metallb.io/v1beta1
kind: BGPAdvertisement
metadata:
  name: %s-bgp-adv
  namespace: metallb-system
spec:
  ipAddressPools:
  - %s-ip-pool
`, cfg.Metadata.Name, cfg.Metadata.Name)

	cmd := fmt.Sprintf(`echo '%s' | kubectl apply -f -`, advYAML)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("创建 BGPAdvertisement 失败: %w", err)
	}

	return nil
}

// generateMetalLBBGPConfig 生成 MetalLB BGP 配置
func generateMetalLBBGPConfig(cfg *config.ClusterConfig) (string, error) {
	params := MetalLBBGPConfig{
		ClusterName:     cfg.Metadata.Name,
		LocalASN:        cfg.Spec.BGP.LocalASN,
		BGPPeers:        cfg.Spec.BGP.Peers,
		LoadBalancerIPs: cfg.Spec.BGP.LoadBalancerIPs,
	}

	tmpl, err := template.New("metallb-bgp").Parse(metallbBGPTemplate)
	if err != nil {
		return "", fmt.Errorf("解析 MetalLB BGP 模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("生成 MetalLB BGP 配置失败: %w", err)
	}

	return buf.String(), nil
}

// verifyMetalLBBGP 验证 MetalLB BGP 配置
func verifyMetalLBBGP(client executor.CommandExecutor) error {
	// 检查 IPAddressPool
	if _, err := client.Execute("kubectl get ipaddresspool -n metallb-system"); err != nil {
		return fmt.Errorf("验证 IPAddressPool 失败: %w", err)
	}

	// 检查 BGPPeer
	if _, err := client.Execute("kubectl get bgppeer -n metallb-system"); err != nil {
		return fmt.Errorf("验证 BGPPeer 失败: %w", err)
	}

	// 检查 BGPAdvertisement
	if _, err := client.Execute("kubectl get bgpadvertisement -n metallb-system"); err != nil {
		return fmt.Errorf("验证 BGPAdvertisement 失败: %w", err)
	}

	return nil
}
