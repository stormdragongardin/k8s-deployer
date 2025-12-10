package binary

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"stormdragon/k8s-deployer/pkg/ui"
)

// BinaryInfo 二进制文件信息
type BinaryInfo struct {
	Name    string
	Version string
	URL     string
	SHA256  string // 可选的校验和
}

// Manager 二进制文件管理器
type Manager struct {
	CacheDir string
}

// NewManager 创建二进制文件管理器
func NewManager(cacheDir string) (*Manager, error) {
	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}
	
	return &Manager{
		CacheDir: cacheDir,
	}, nil
}

// GetBinaryPath 获取二进制文件路径（如果不存在则下载）
func (m *Manager) GetBinaryPath(info BinaryInfo) (string, error) {
	// 构建缓存路径
	cachePath := filepath.Join(m.CacheDir, info.Name, info.Version, filepath.Base(info.URL))
	
	// 检查是否已缓存
	if _, err := os.Stat(cachePath); err == nil {
		ui.Info("使用缓存的 %s %s", info.Name, info.Version)
		return cachePath, nil
	}
	
	// 下载文件
	ui.Info("下载 %s %s...", info.Name, info.Version)
	if err := m.downloadBinary(info, cachePath); err != nil {
		return "", err
	}
	
	return cachePath, nil
}

// downloadBinary 下载二进制文件
func (m *Manager) downloadBinary(info BinaryInfo, destPath string) error {
	// 创建目标目录
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	
	// 创建临时文件
	tmpFile := destPath + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()
	
	// 发起 HTTP 请求
	client := &http.Client{
		Timeout: 30 * time.Minute, // 大文件下载超时时间
	}
	
	resp, err := client.Get(info.URL)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpFile)
		return fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}
	
	// 显示下载进度
	ui.Info("下载中... (大小: %d MB)", resp.ContentLength/1024/1024)
	
	// 复制数据并计算 SHA256
	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	
	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("下载数据失败: %w", err)
	}
	
	ui.Success("下载完成: %d MB", written/1024/1024)
	
	// 校验 SHA256（如果提供）
	if info.SHA256 != "" {
		actualHash := fmt.Sprintf("%x", hash.Sum(nil))
		if actualHash != info.SHA256 {
			os.Remove(tmpFile)
			return fmt.Errorf("SHA256 校验失败，期望: %s, 实际: %s", info.SHA256, actualHash)
		}
		ui.Success("SHA256 校验通过")
	}
	
	// 重命名为最终文件
	if err := os.Rename(tmpFile, destPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("重命名文件失败: %w", err)
	}
	
	// 设置可执行权限（对于二进制文件）
	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("设置权限失败: %w", err)
	}
	
	return nil
}

// GetKubernetesVersion 获取 Kubernetes 版本的下载信息
func GetKubernetesVersion(version string) []BinaryInfo {
	baseURL := fmt.Sprintf("https://dl.k8s.io/release/%s/bin/linux/amd64", version)
	
	return []BinaryInfo{
		{
			Name:    "kubectl",
			Version: version,
			URL:     baseURL + "/kubectl",
		},
		{
			Name:    "kubeadm",
			Version: version,
			URL:     baseURL + "/kubeadm",
		},
		{
			Name:    "kubelet",
			Version: version,
			URL:     baseURL + "/kubelet",
		},
	}
}

// GetContainerdInfo 获取 containerd 下载信息
func GetContainerdInfo(version string) BinaryInfo {
	return BinaryInfo{
		Name:    "containerd",
		Version: version,
		URL:     fmt.Sprintf("https://github.com/containerd/containerd/releases/download/v%s/containerd-%s-linux-amd64.tar.gz", version, version),
	}
}

// GetHelmInfo 获取 Helm 下载信息
func GetHelmInfo(version string) BinaryInfo {
	return BinaryInfo{
		Name:    "helm",
		Version: version,
		URL:     fmt.Sprintf("https://get.helm.sh/helm-v%s-linux-amd64.tar.gz", version),
	}
}

// CleanCache 清理缓存
func (m *Manager) CleanCache() error {
	ui.Info("清理缓存目录: %s", m.CacheDir)
	return os.RemoveAll(m.CacheDir)
}

// ListCached 列出已缓存的文件
func (m *Manager) ListCached() ([]string, error) {
	var cached []string
	
	err := filepath.Walk(m.CacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(m.CacheDir, path)
			cached = append(cached, relPath)
		}
		return nil
	})
	
	return cached, err
}

