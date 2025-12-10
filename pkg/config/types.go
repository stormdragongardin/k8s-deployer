package config

// ClusterConfig 集群配置
type ClusterConfig struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   MetadataConfig  `yaml:"metadata"`
	Spec       ClusterSpec     `yaml:"spec"`
}

// MetadataConfig 元数据配置
type MetadataConfig struct {
	Name string `yaml:"name"`
}

// ClusterSpec 集群规格配置
type ClusterSpec struct {
	Version         string              `yaml:"version"`          // Kubernetes 版本
	ImageRepository string              `yaml:"imageRepository"`  // Harbor 镜像仓库地址
	Harbor          HarborConfig        `yaml:"harbor"`           // Harbor 认证配置
	Networking      NetworkConfig       `yaml:"networking"`       // 网络配置
	HA              HAConfig            `yaml:"ha"`               // 高可用配置
	Hubble          HubbleConfig        `yaml:"hubble"`           // Hubble 可观测性配置
	LoadBalancer    LoadBalancerConfig  `yaml:"loadBalancer"`     // LoadBalancer 配置
	BGP             BGPConfig           `yaml:"bgp"`              // BGP 配置
	GatewayAPI      GatewayAPIConfig    `yaml:"gatewayAPI"`       // Gateway API 配置
	Envoy           EnvoyConfig         `yaml:"envoy"`            // Envoy L7 代理配置
	Nodes           []NodeConfig        `yaml:"nodes"`            // 节点配置
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	PodSubnet     string `yaml:"podSubnet"`     // Pod 网段
	ServiceSubnet string `yaml:"serviceSubnet"` // Service 网段
}

// HAConfig 高可用配置
type HAConfig struct {
	Enabled bool   `yaml:"enabled"` // 是否启用高可用
	VIP     string `yaml:"vip"`     // 虚拟 IP
}

// HarborConfig Harbor 认证配置
type HarborConfig struct {
	Username string `yaml:"username"` // Harbor 用户名（可选）
	Password string `yaml:"password"` // Harbor 密码（可选）
	Insecure bool   `yaml:"insecure"` // 是否跳过 TLS 验证（默认 false）
}

// BGPConfig BGP 配置
type BGPConfig struct {
	Enabled         bool            `yaml:"enabled"`         // 是否启用 BGP
	LocalASN        int             `yaml:"localASN"`        // 本地 AS 号
	Peers           []BGPPeerConfig `yaml:"peers"`           // BGP 对等体列表
	LoadBalancerIPs []string        `yaml:"loadBalancerIPs"` // LoadBalancer IP 池
}

// BGPPeerConfig BGP 对等体配置
type BGPPeerConfig struct {
	PeerAddress string `yaml:"peerAddress"` // 对等体 IP
	PeerASN     int    `yaml:"peerASN"`     // 对等体 AS 号
}

// HubbleConfig Hubble 可观测性配置
type HubbleConfig struct {
	Enabled bool           `yaml:"enabled"` // 是否启用 Hubble
	Metrics HubbleMetrics  `yaml:"metrics"` // Hubble 指标配置
	UI      HubbleUIConfig `yaml:"ui"`      // Hubble UI 配置
}

// HubbleMetrics Hubble 指标配置
type HubbleMetrics struct {
	Enabled bool `yaml:"enabled"` // 是否启用 Hubble 指标
}

// HubbleUIConfig Hubble UI 配置
type HubbleUIConfig struct {
	Enabled  bool `yaml:"enabled"`  // 是否启用 Hubble UI
	NodePort int  `yaml:"nodePort"` // NodePort 端口（可选）
}

// LoadBalancerConfig LoadBalancer 配置
type LoadBalancerConfig struct {
	Provider string `yaml:"provider"` // 提供者: cilium (默认 cilium)
	Mode     string `yaml:"mode"`     // 模式: dsr, snat (默认 dsr)
}

// GatewayAPIConfig Gateway API 配置
type GatewayAPIConfig struct {
	Enabled bool `yaml:"enabled"` // 是否启用 Gateway API
}

// EnvoyConfig Envoy L7 代理配置
type EnvoyConfig struct {
	Enabled bool `yaml:"enabled"` // 是否启用 Envoy (Gateway API 需要)
}

// NodeConfig 节点配置
type NodeConfig struct {
	Role     string    `yaml:"role"`     // 角色: master / worker
	IP       string    `yaml:"ip"`       // IP 地址
	Hostname string    `yaml:"hostname"` // 主机名（可选，自动生成）
	GPU      bool      `yaml:"gpu"`      // 是否为 GPU 节点
	SSH      SSHConfig `yaml:"ssh"`      // SSH 配置
}

// SSHConfig SSH 连接配置
type SSHConfig struct {
	User     string `yaml:"user"`     // SSH 用户名
	Port     int    `yaml:"port"`     // SSH 端口
	KeyFile  string `yaml:"keyFile"`  // SSH 私钥文件路径（可选）
	Password string `yaml:"password"` // SSH 密码（可选，不推荐）
}

// DefaultConfig 返回默认配置
func DefaultConfig() *ClusterConfig {
	return &ClusterConfig{
		APIVersion: "k8s-deployer/v1",
		Kind:       "Cluster",
		Spec: ClusterSpec{
			Version:         "v1.34.2",
			ImageRepository: "registry.k8s.io",
			Networking: NetworkConfig{
				PodSubnet:     "10.244.0.0/16",
				ServiceSubnet: "10.96.0.0/12",
			},
			HA: HAConfig{
				Enabled: false,
			},
		},
	}
}

