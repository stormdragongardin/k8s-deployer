package cluster

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

const (
	// DeployerLabel 标识由 k8s-deployer 管理的集群
	DeployerLabel = "k8s-deployer.stormdragon.io/managed"
	// DeployerVersion 工具版本标签
	DeployerVersion = "k8s-deployer.stormdragon.io/version"
	// DeployerConfigMap 配置存储的 ConfigMap 名称
	DeployerConfigMap = "k8s-deployer-config"
	// DeployerSecret 敏感信息存储的 Secret 名称
	DeployerSecret = "k8s-deployer-secret"
	// DeployerNamespace 部署器资源所在的命名空间
	DeployerNamespace = "kube-system"
	// DeployerToolVersion 当前工具版本
	DeployerToolVersion = "v1.0.0"
)

// SaveClusterConfig 保存集群配置到 ConfigMap 和 Secret
func SaveClusterConfig(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	ui.Header("保存集群配置")

	// 1. 标记集群节点
	ui.Step(1, 3, "标记集群节点")
	if err := labelClusterNodes(client, cfg); err != nil {
		ui.Warning("标记节点失败: %v", err)
	} else {
		ui.Success("节点标记完成")
	}

	// 2. 保存非敏感配置到 ConfigMap
	ui.Step(2, 3, "保存集群配置")
	if err := saveConfigToConfigMap(client, cfg); err != nil {
		return fmt.Errorf("保存配置到 ConfigMap 失败: %w", err)
	}

	// 3. 保存敏感信息到 Secret
	ui.Step(3, 3, "保存敏感信息")
	if err := saveSensitiveToSecret(client, cfg); err != nil {
		ui.Warning("保存敏感信息失败: %v（不影响集群使用）", err)
	}

	ui.Success("集群配置保存完成")
	return nil
}

// labelClusterNodes 为所有节点打上 k8s-deployer 标签
func labelClusterNodes(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	for _, node := range cfg.Spec.Nodes {
		labels := fmt.Sprintf("%s=true,%s=%s", DeployerLabel, DeployerVersion, DeployerToolVersion)
		cmd := fmt.Sprintf("kubectl label node %s %s --overwrite", node.Hostname, labels)

		if _, err := client.Execute(cmd); err != nil {
			return fmt.Errorf("标记节点 %s 失败: %w", node.Hostname, err)
		}

		ui.SubStep("✓ 节点 %s 已标记", node.Hostname)
	}
	return nil
}

// saveConfigToConfigMap 保存配置到 ConfigMap（不含敏感信息）
func saveConfigToConfigMap(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	// 创建配置副本，清除敏感信息
	cfgCopy := *cfg
	cfgCopy.Spec.Harbor.Username = ""
	cfgCopy.Spec.Harbor.Password = ""
	for i := range cfgCopy.Spec.Nodes {
		cfgCopy.Spec.Nodes[i].SSH.Password = ""
	}

	// 序列化为 YAML
	data, err := yaml.Marshal(&cfgCopy)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 缩进 YAML 数据（用于嵌入 ConfigMap）
	indentedData := indentYAML(string(data), 4)

	// 创建 ConfigMap YAML
	now := time.Now().Format(time.RFC3339)
	configMapYAML := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    app: k8s-deployer
    cluster: %s
    %s: "true"
    %s: %s
  annotations:
    k8s-deployer.stormdragon.io/deployed-at: "%s"
    k8s-deployer.stormdragon.io/deployed-by: "k8s-deployer"
data:
  cluster.yaml: |
%s`, DeployerConfigMap, DeployerNamespace, cfg.Metadata.Name,
		DeployerLabel, DeployerVersion, DeployerToolVersion,
		now, indentedData)

	// 应用 ConfigMap
	cmd := fmt.Sprintf("cat > /tmp/deployer-config.yaml << 'EOF'\n%s\nEOF", configMapYAML)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("创建配置文件失败: %w", err)
	}

	if _, err := client.Execute("kubectl apply -f /tmp/deployer-config.yaml"); err != nil {
		return fmt.Errorf("应用 ConfigMap 失败: %w", err)
	}

	ui.SubStep("✓ 配置已保存到 ConfigMap: %s/%s", DeployerNamespace, DeployerConfigMap)
	return nil
}

// saveSensitiveToSecret 保存敏感信息到 Secret
func saveSensitiveToSecret(client *executor.SSHClient, cfg *config.ClusterConfig) error {
	// 只有在有敏感信息时才创建 Secret
	if cfg.Spec.Harbor.Username == "" && cfg.Spec.Harbor.Password == "" {
		ui.SubStep("无敏感信息，跳过 Secret 创建")
		return nil
	}

	now := time.Now().Format(time.RFC3339)
	secretYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    app: k8s-deployer
    cluster: %s
    %s: "true"
    %s: %s
  annotations:
    k8s-deployer.stormdragon.io/created-at: "%s"
type: Opaque
stringData:
  harbor-username: "%s"
  harbor-password: "%s"
`, DeployerSecret, DeployerNamespace, cfg.Metadata.Name,
		DeployerLabel, DeployerVersion, DeployerToolVersion,
		now, cfg.Spec.Harbor.Username, cfg.Spec.Harbor.Password)

	cmd := fmt.Sprintf("cat > /tmp/deployer-secret.yaml << 'EOF'\n%s\nEOF", secretYAML)
	if _, err := client.Execute(cmd); err != nil {
		return fmt.Errorf("创建 Secret 文件失败: %w", err)
	}

	if _, err := client.Execute("kubectl apply -f /tmp/deployer-secret.yaml"); err != nil {
		return fmt.Errorf("应用 Secret 失败: %w", err)
	}

	ui.SubStep("✓ 敏感信息已保存到 Secret: %s/%s", DeployerNamespace, DeployerSecret)
	return nil
}

// LoadClusterConfig 从 ConfigMap 和 Secret 加载集群配置
func LoadClusterConfig(client executor.CommandExecutor, clusterName string) (*config.ClusterConfig, error) {
	// 1. 检查集群是否由 k8s-deployer 管理
	if err := verifyDeployerManaged(client); err != nil {
		return nil, err
	}

	// 2. 从 ConfigMap 读取配置
	output, err := client.Execute(fmt.Sprintf(
		"kubectl get configmap %s -n %s -o jsonpath='{.data.cluster\\.yaml}'",
		DeployerConfigMap, DeployerNamespace))
	if err != nil {
		return nil, fmt.Errorf("无法获取集群配置: %w\n提示: 此集群可能不是通过 k8s-deployer 部署的", err)
	}

	// 3. 解析 YAML
	var cfg config.ClusterConfig
	if err := yaml.Unmarshal([]byte(output), &cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 4. 尝试加载敏感信息（如果存在）
	loadSensitiveInfo(client, &cfg)

	return &cfg, nil
}

// verifyDeployerManaged 验证集群是否由 k8s-deployer 管理
func verifyDeployerManaged(client executor.CommandExecutor) error {
	// 检查是否有带 k8s-deployer 标签的节点
	// 使用 kubectl 原生方式统计，不依赖 wc（Windows 不支持）
	cmd := fmt.Sprintf("kubectl get nodes -l %s=true --no-headers 2>/dev/null", DeployerLabel)
	output, err := client.Execute(cmd)
	if err != nil {
		return fmt.Errorf("无法检查集群标签: %w", err)
	}

	// 检查输出是否为空（没有节点）
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("此集群不是通过 k8s-deployer 部署的\n" +
			"提示: 集群节点缺少标签 '%s=true'\n" +
			"只有通过 k8s-deployer 部署的集群才能使用 update 命令", DeployerLabel)
	}

	return nil
}

// loadSensitiveInfo 加载敏感信息（不影响主流程）
func loadSensitiveInfo(client executor.CommandExecutor, cfg *config.ClusterConfig) {
	// 尝试读取 Harbor 用户名
	if username, err := client.Execute(fmt.Sprintf(
		"kubectl get secret %s -n %s -o jsonpath='{.data.harbor-username}' 2>/dev/null | base64 -d",
		DeployerSecret, DeployerNamespace)); err == nil && username != "" {
		cfg.Spec.Harbor.Username = username
	}

	// 尝试读取 Harbor 密码
	if password, err := client.Execute(fmt.Sprintf(
		"kubectl get secret %s -n %s -o jsonpath='{.data.harbor-password}' 2>/dev/null | base64 -d",
		DeployerSecret, DeployerNamespace)); err == nil && password != "" {
		cfg.Spec.Harbor.Password = password
	}
}

// UpdateClusterConfigMap 更新 ConfigMap 中的配置
func UpdateClusterConfigMap(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	// 创建配置副本，清除敏感信息
	cfgCopy := *cfg
	cfgCopy.Spec.Harbor.Username = ""
	cfgCopy.Spec.Harbor.Password = ""
	for i := range cfgCopy.Spec.Nodes {
		cfgCopy.Spec.Nodes[i].SSH.Password = ""
	}

	// 序列化为 YAML
	data, err := yaml.Marshal(&cfgCopy)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 转义 YAML 用于 JSON patch
	escapedYAML := strings.ReplaceAll(string(data), `"`, `\"`)
	escapedYAML = strings.ReplaceAll(escapedYAML, "\n", "\\n")

	// 更新 ConfigMap
	now := time.Now().Format(time.RFC3339)
	patchData := fmt.Sprintf(`{"data": {"cluster.yaml": "%s"}, "metadata": {"annotations": {"k8s-deployer.stormdragon.io/updated-at": "%s"}}}`,
		escapedYAML, now)

	// 使用临时文件方式（跨平台兼容）
	// Windows: $env:TEMP, Unix: /tmp
	tmpFile := "$env:TEMP\\k8s-deployer-patch.json"
	
	// 创建临时 patch 文件（PowerShell 使用 Out-File）
	escapedPatch := strings.ReplaceAll(patchData, "'", "''")
	writeCmd := fmt.Sprintf(`@'
%s
'@ | Out-File -FilePath %s -Encoding UTF8 -NoNewline`, escapedPatch, tmpFile)
	
	if _, err := client.Execute(writeCmd); err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	// 应用 patch（使用完整路径）
	patchCmd := fmt.Sprintf("kubectl patch configmap %s -n %s --type=merge --patch-file=%s",
		DeployerConfigMap, DeployerNamespace, tmpFile)
	
	if _, err := client.Execute(patchCmd); err != nil {
		// 清理临时文件
		client.Execute(fmt.Sprintf("Remove-Item -Path %s -ErrorAction SilentlyContinue", tmpFile))
		return fmt.Errorf("更新 ConfigMap 失败: %w", err)
	}

	// 清理临时文件
	client.Execute(fmt.Sprintf("Remove-Item -Path %s -ErrorAction SilentlyContinue", tmpFile))

	return nil
}

// indentYAML 缩进 YAML 内容
func indentYAML(content string, spaces int) string {
	lines := strings.Split(content, "\n")
	indent := strings.Repeat(" ", spaces)

	var result strings.Builder
	for i, line := range lines {
		if line != "" {
			result.WriteString(indent)
			result.WriteString(line)
		}
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// GetClusterInfo 获取集群部署信息
func GetClusterInfo(client *executor.SSHClient) (map[string]string, error) {
	info := make(map[string]string)

	// 获取部署时间
	if output, err := client.Execute(fmt.Sprintf(
		"kubectl get configmap %s -n %s -o jsonpath='{.metadata.annotations.k8s-deployer\\.stormdragon\\.io/deployed-at}'",
		DeployerConfigMap, DeployerNamespace)); err == nil {
		info["deployed-at"] = output
	}

	// 获取更新时间
	if output, err := client.Execute(fmt.Sprintf(
		"kubectl get configmap %s -n %s -o jsonpath='{.metadata.annotations.k8s-deployer\\.stormdragon\\.io/updated-at}'",
		DeployerConfigMap, DeployerNamespace)); err == nil && output != "" {
		info["updated-at"] = output
	}

	// 获取工具版本
	if output, err := client.Execute(fmt.Sprintf(
		"kubectl get nodes -l %s -o jsonpath='{.items[0].metadata.labels.k8s-deployer\\.stormdragon\\.io/version}'",
		DeployerLabel)); err == nil {
		info["tool-version"] = output
	}

	// 获取集群名称
	if output, err := client.Execute(fmt.Sprintf(
		"kubectl get configmap %s -n %s -o jsonpath='{.metadata.labels.cluster}'",
		DeployerConfigMap, DeployerNamespace)); err == nil {
		info["cluster-name"] = output
	}

	return info, nil
}

