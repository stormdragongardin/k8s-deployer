package kubeadm

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"stormdragon/k8s-deployer/pkg/config"
)

//go:embed templates/kubeadm-init.yaml.tpl
var kubeadmInitTemplate string

// InitConfig kubeadm init 配置参数
type InitConfig struct {
	Version              string
	ImageRepository      string
	ControlPlaneEndpoint string
	ClusterName          string
	VIP                  string
	LocalIP              string
	PodSubnet            string
	ServiceSubnet        string
	MasterIPs            []string
}

// GenerateInitConfig 生成 kubeadm init 配置
func GenerateInitConfig(clusterConfig *config.ClusterConfig, localIP string) (string, error) {
	// 收集所有 master 节点 IP
	var masterIPs []string
	for _, node := range clusterConfig.Spec.Nodes {
		if node.Role == "master" {
			masterIPs = append(masterIPs, node.IP)
		}
	}

	// 确定控制平面端点
	controlPlaneEndpoint := localIP + ":6443"
	if clusterConfig.Spec.HA.Enabled {
		controlPlaneEndpoint = clusterConfig.Spec.HA.VIP + ":6443"
	}

	// 构建配置参数
	params := InitConfig{
		Version:              clusterConfig.Spec.Version,
		ImageRepository:      clusterConfig.Spec.ImageRepository,
		ControlPlaneEndpoint: controlPlaneEndpoint,
		ClusterName:          clusterConfig.Metadata.Name,
		VIP:                  clusterConfig.Spec.HA.VIP,
		LocalIP:              localIP,
		PodSubnet:            clusterConfig.Spec.Networking.PodSubnet,
		ServiceSubnet:        clusterConfig.Spec.Networking.ServiceSubnet,
		MasterIPs:            masterIPs,
	}

	// 渲染模板
	tmpl, err := template.New("kubeadm-init").Parse(kubeadmInitTemplate)
	if err != nil {
		return "", fmt.Errorf("解析模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("渲染模板失败: %w", err)
	}

	return buf.String(), nil
}

// JoinCommand join 命令结构
type JoinCommand struct {
	APIServerEndpoint string
	Token             string
	CertificateKey    string // 仅 master 节点需要
	CACertHash        string
}

// GenerateMasterJoinCommand 生成 master 节点 join 命令
func GenerateMasterJoinCommand(cmd *JoinCommand) string {
	return fmt.Sprintf(`kubeadm join %s \
  --token %s \
  --discovery-token-ca-cert-hash %s \
  --control-plane \
  --certificate-key %s \
  --cri-socket unix:///var/run/containerd/containerd.sock`,
		cmd.APIServerEndpoint,
		cmd.Token,
		cmd.CACertHash,
		cmd.CertificateKey,
	)
}

// GenerateWorkerJoinCommand 生成 worker 节点 join 命令
func GenerateWorkerJoinCommand(cmd *JoinCommand) string {
	return fmt.Sprintf(`kubeadm join %s \
  --token %s \
  --discovery-token-ca-cert-hash %s \
  --cri-socket unix:///var/run/containerd/containerd.sock`,
		cmd.APIServerEndpoint,
		cmd.Token,
		cmd.CACertHash,
	)
}

// GetInitCommand 获取 kubeadm init 命令
// skipPhases: 要跳过的阶段，例如 "addon/kube-proxy"
func GetInitCommand(configFile string, skipPhases []string) string {
	cmd := fmt.Sprintf("kubeadm init --config %s", configFile)
	
	if len(skipPhases) > 0 {
		for _, phase := range skipPhases {
			cmd += fmt.Sprintf(" --skip-phases=%s", phase)
		}
	}
	
	return cmd
}

// GetResetCommand 获取 kubeadm reset 命令
func GetResetCommand() string {
	return "kubeadm reset -f --cri-socket unix:///var/run/containerd/containerd.sock"
}

