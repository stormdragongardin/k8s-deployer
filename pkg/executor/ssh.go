package executor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHClient SSH 客户端
type SSHClient struct {
	Host   string
	Port   int
	User   string
	client *ssh.Client
	// 保留原始认证信息，用于降级重连
	keyFile  string
	password string
}

// NewSSHClient 创建新的 SSH 客户端
// 支持密钥认证或密码认证
func NewSSHClient(host string, port int, user, keyFile string) (*SSHClient, error) {
	return NewSSHClientWithPassword(host, port, user, keyFile, "")
}

// NewSSHClientWithPassword 创建新的 SSH 客户端（支持密码）
func NewSSHClientWithPassword(host string, port int, user, keyFile, password string) (*SSHClient, error) {
	config := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 生产环境应该验证 host key
		Timeout:         30 * time.Second,
	}
	
	// 优先使用密钥认证
	if keyFile != "" {
		keyPath := expandPath(keyFile)
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥文件失败: %w", err)
		}
		
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %w", err)
		}
		
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if password != "" {
		// 使用密码认证
		config.Auth = []ssh.AuthMethod{ssh.Password(password)}
	} else {
		return nil, fmt.Errorf("必须提供 SSH 密钥或密码")
	}
	
	// 连接
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	
	return &SSHClient{
		Host:     host,
		Port:     port,
		User:     user,
		client:   client,
		keyFile:  keyFile,
		password: password,
	}, nil
}

// Execute 执行远程命令
func (c *SSHClient) Execute(command string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建 SSH session 失败: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	if err != nil {
		return "", fmt.Errorf("命令执行失败: %w\n标准错误: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// ExecuteWithOutput 执行命令并实时输出
func (c *SSHClient) ExecuteWithOutput(command string, output io.Writer) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建 SSH session 失败: %w", err)
	}
	defer session.Close()

	session.Stdout = output
	session.Stderr = output

	return session.Run(command)
}

// UploadFile 上传文件到远程服务器
func (c *SSHClient) UploadFile(localPath, remotePath string) error {
	// 读取本地文件
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("读取本地文件失败: %w", err)
	}

	// 获取文件权限
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}
	mode := fileInfo.Mode().Perm()

	// 创建远程目录
	remoteDir := filepath.Dir(remotePath)
	if _, err := c.Execute(fmt.Sprintf("mkdir -p %s", remoteDir)); err != nil {
		return fmt.Errorf("创建远程目录失败: %w", err)
	}

	// 使用 SCP 上传文件
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建 SSH session 失败: %w", err)
	}
	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		
		// SCP 协议
		fmt.Fprintf(w, "C%#o %d %s\n", mode, len(data), filepath.Base(remotePath))
		w.Write(data)
		fmt.Fprint(w, "\x00")
	}()

	// 执行 SCP 命令
	if err := session.Run(fmt.Sprintf("scp -t %s", remotePath)); err != nil {
		return fmt.Errorf("SCP 上传失败: %w", err)
	}

	return nil
}

// DownloadFile 从远程服务器下载文件
func (c *SSHClient) DownloadFile(remotePath, localPath string) error {
	// 读取远程文件内容
	content, err := c.Execute(fmt.Sprintf("cat %s", remotePath))
	if err != nil {
		return fmt.Errorf("读取远程文件失败: %w", err)
	}

	// 创建本地目录
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	// 写入本地文件
	if err := os.WriteFile(localPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入本地文件失败: %w", err)
	}

	return nil
}

// FileExists 检查远程文件是否存在
func (c *SSHClient) FileExists(path string) (bool, error) {
	_, err := c.Execute(fmt.Sprintf("test -f %s", path))
	if err != nil {
		if err.Error() == "Process exited with status 1" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DirExists 检查远程目录是否存在
func (c *SSHClient) DirExists(path string) (bool, error) {
	_, err := c.Execute(fmt.Sprintf("test -d %s", path))
	if err != nil {
		if err.Error() == "Process exited with status 1" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Close 关闭 SSH 连接
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// expandPath 展开路径中的 ~ 为用户主目录
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// TestConnection 测试 SSH 连接
func TestConnection(host string, port int, user, keyFile string) error {
	return TestConnectionWithPassword(host, port, user, keyFile, "")
}

// TestConnectionWithPassword 测试 SSH 连接（支持密码）
func TestConnectionWithPassword(host string, port int, user, keyFile, password string) error {
	client, err := NewSSHClientWithPassword(host, port, user, keyFile, password)
	if err != nil {
		return err
	}
	defer client.Close()

	// 执行简单命令测试连接
	_, err = client.Execute("echo 'test'")
	return err
}

// ExecuteLocalCommand 执行本地命令
func ExecuteLocalCommand(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("命令执行失败: %w\n输出: %s\n错误: %s", 
			err, stdout.String(), stderr.String())
	}
	
	return stdout.String(), nil
}

// NewSSHClientSmart 智能创建 SSH 客户端，支持自动降级
// 尝试顺序：
// 1. root + SSH 密钥（如果已配置）
// 2. 原始用户 + 密码（降级方案）
func NewSSHClientSmart(host string, port int, user, keyFile, password string) (*SSHClient, error) {
	// 尝试 1: root + SSH 密钥（假设已提权）
	rootKeyFile := "/root/.ssh/id_rsa"
	if keyFile == "" {
		keyFile = rootKeyFile
	}
	
	client, err := NewSSHClientWithPassword(host, port, "root", rootKeyFile, "")
	if err == nil {
		// root 密钥连接成功
		// 但保留原始认证信息，以备后续降级使用
		client.keyFile = keyFile
		client.password = password
		return client, nil
	}
	
	// 尝试 2: 原始用户 + 密钥（如果提供了）
	if keyFile != "" && keyFile != rootKeyFile {
		client, err = NewSSHClientWithPassword(host, port, user, keyFile, "")
		if err == nil {
			client.password = password
			return client, nil
		}
	}
	
	// 尝试 3: 原始用户 + 密码（降级方案）
	if password != "" {
		client, err = NewSSHClientWithPassword(host, port, user, "", password)
		if err == nil {
			client.keyFile = keyFile
			return client, nil
		}
	}
	
	return nil, fmt.Errorf("所有 SSH 连接方式均失败: root 密钥、用户密钥、用户密码")
}

// ExecuteWithSudo 执行需要 root 权限的命令，自动处理提权
// 如果当前是 root 用户，直接执行
// 如果当前是普通用户，使用 sudo 提权
func (c *SSHClient) ExecuteWithSudo(command string) (string, error) {
	// 检查当前用户
	currentUser, err := c.Execute("whoami")
	if err == nil && strings.TrimSpace(currentUser) == "root" {
		// 已经是 root，直接执行
		return c.Execute(command)
	}
	
	// 需要 sudo 提权
	if c.password != "" {
		// 使用密码 sudo
		sudoCmd := fmt.Sprintf("echo '%s' | sudo -S bash -c '%s'", 
			c.password, 
			strings.ReplaceAll(command, "'", "'\\''"))
		return c.Execute(sudoCmd)
	}
	
	// 尝试无密码 sudo
	return c.Execute(fmt.Sprintf("sudo bash -c '%s'", 
		strings.ReplaceAll(command, "'", "'\\'")))
}

// Reconnect 重新连接（用于连接失效时）
func (c *SSHClient) Reconnect() error {
	// 关闭旧连接
	if c.client != nil {
		c.client.Close()
	}
	
	// 尝试重新连接
	newClient, err := NewSSHClientSmart(c.Host, c.Port, c.User, c.keyFile, c.password)
	if err != nil {
		return fmt.Errorf("重新连接失败: %w", err)
	}
	
	c.client = newClient.client
	c.User = newClient.User
	return nil
}

// ExecuteWithRetry 执行命令，失败时自动重试（可能涉及重连）
func (c *SSHClient) ExecuteWithRetry(command string, retries int) (string, error) {
	var lastErr error
	
	for i := 0; i < retries; i++ {
		output, err := c.Execute(command)
		if err == nil {
			return output, nil
		}
		
		lastErr = err
		
		// 检查是否是连接错误
		if strings.Contains(err.Error(), "connection") || 
		   strings.Contains(err.Error(), "broken pipe") ||
		   strings.Contains(err.Error(), "EOF") {
			// 尝试重新连接
			if reconnectErr := c.Reconnect(); reconnectErr != nil {
				continue
			}
			// 重连成功，再试一次
			output, err = c.Execute(command)
			if err == nil {
				return output, nil
			}
			lastErr = err
		}
		
		// 短暂延迟后重试
		if i < retries-1 {
			time.Sleep(2 * time.Second)
		}
	}
	
	return "", fmt.Errorf("命令执行失败（重试 %d 次）: %w", retries, lastErr)
}

