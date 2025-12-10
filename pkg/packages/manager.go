package packages

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager 包管理器
type Manager struct {
	PackageDir string // packages 目录路径
	K8sVersion string // Kubernetes 版本
}

// NewManager 创建包管理器
func NewManager() *Manager {
	// 获取当前工作目录
	cwd, _ := os.Getwd()
	return &Manager{
		PackageDir: filepath.Join(cwd, "packages"),
		K8sVersion: "v1.34.2", // 默认版本
	}
}

// NewManagerWithVersion 创建指定版本的包管理器
func NewManagerWithVersion(k8sVersion string) *Manager {
	cwd, _ := os.Getwd()
	return &Manager{
		PackageDir: filepath.Join(cwd, "packages"),
		K8sVersion: k8sVersion,
	}
}

// GetPackagePath 获取包的完整路径
func (m *Manager) GetPackagePath(pkgName string) string {
	var relPath string

	switch pkgName {
	case "containerd":
		relPath = "containerd/containerd-2.2.0-linux-amd64.tar.gz"
	case "runc":
		relPath = "containerd/runc.amd64"
	case "cni-plugins":
		relPath = "containerd/cni-plugins-linux-amd64-v1.8.0.tgz"
	case "kubectl":
		relPath = fmt.Sprintf("kubernetes/%s/kubectl", m.K8sVersion)
	case "kubeadm":
		relPath = fmt.Sprintf("kubernetes/%s/kubeadm", m.K8sVersion)
	case "kubelet":
		relPath = fmt.Sprintf("kubernetes/%s/kubelet", m.K8sVersion)
	case "helm":
		relPath = "helm/linux-amd64/helm"
	case "cilium-chart":
		relPath = "cilium/cilium-1.18.4.tgz"
	case "metallb-chart":
		relPath = "metallb/metallb-0.15.2.tgz"
	default:
		return ""
	}

	return filepath.Join(m.PackageDir, relPath)
}

// Exists 检查包是否存在
func (m *Manager) Exists(pkgName string) bool {
	path := m.GetPackagePath(pkgName)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// CheckRequiredPackages 检查必需的包是否都存在
func (m *Manager) CheckRequiredPackages(required []string) []string {
	var missing []string

	for _, pkg := range required {
		if !m.Exists(pkg) {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// ListAvailable 列出所有可用的包
func (m *Manager) ListAvailable() []string {
	var available []string

	pkgNames := []string{"containerd", "runc", "cni-plugins", "kubectl", "kubeadm", "kubelet", "helm"}
	for _, name := range pkgNames {
		if m.Exists(name) {
			available = append(available, name)
		}
	}

	return available
}

// GetPackageInfo 获取包信息（用于显示）
func (m *Manager) GetPackageInfo(pkgName string) string {
	path := m.GetPackagePath(pkgName)
	if path == "" {
		return fmt.Sprintf("%s: 未知包", pkgName)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("%s: 不存在 (路径: %s)", pkgName, path)
	}

	sizeMB := float64(info.Size()) / 1024 / 1024
	return fmt.Sprintf("%s: %.2f MB", pkgName, sizeMB)
}
