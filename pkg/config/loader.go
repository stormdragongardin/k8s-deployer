package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFromFile 从 YAML 文件加载配置
func LoadFromFile(path string) (*ClusterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证配置（使用增强的验证器）
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// 处理节点主机名
	processNodeHostnames(config)

	return config, nil
}

// LoadConfig 是 LoadFromFile 的别名
func LoadConfig(path string) (*ClusterConfig, error) {
	return LoadFromFile(path)
}

// processNodeHostnames 处理节点主机名
// 规则：{cluster-name}-master-{序号}、{cluster-name}-node-{序号}、{cluster-name}-gpu-node-{序号}
func processNodeHostnames(config *ClusterConfig) {
	masterCount := 1
	workerCount := 1
	gpuWorkerCount := 1
	clusterName := config.Metadata.Name

	for i := range config.Spec.Nodes {
		node := &config.Spec.Nodes[i]
		
		// 如果用户没有指定主机名，自动生成
		if node.Hostname == "" {
			if node.Role == "master" {
				node.Hostname = fmt.Sprintf("%s-master-%02d", clusterName, masterCount)
				masterCount++
			} else if node.Role == "worker" {
				if node.GPU {
					node.Hostname = fmt.Sprintf("%s-gpu-node-%02d", clusterName, gpuWorkerCount)
					gpuWorkerCount++
				} else {
					node.Hostname = fmt.Sprintf("%s-node-%02d", clusterName, workerCount)
					workerCount++
				}
			}
		}
	}
}

// ExpandHomePath 扩展家目录路径
func ExpandHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// GetConfigDir 获取配置目录
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	
	configDir := filepath.Join(home, ".k8s-deployer")
	
	// 确保配置目录存在
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	
	// 创建子目录
	dirs := []string{"binaries", "kubeconfigs", "logs"}
	for _, dir := range dirs {
		dirPath := filepath.Join(configDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return "", err
		}
	}
	
	return configDir, nil
}

