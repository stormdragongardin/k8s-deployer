package executor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// LocalExecutor 本地命令执行器
type LocalExecutor struct{}

// NewLocalExecutor 创建本地执行器
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

// Execute 在本地执行命令
func (e *LocalExecutor) Execute(command string) (string, error) {
	var cmd *exec.Cmd
	
	// 根据操作系统选择不同的 shell
	if runtime.GOOS == "windows" {
		// Windows 使用 PowerShell
		cmd = exec.Command("powershell", "-Command", command)
	} else {
		// Unix/Linux 使用 sh
		cmd = exec.Command("sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("命令执行失败: %w\n标准错误: %s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// ExecuteWithOutput 执行命令并实时输出
func (e *LocalExecutor) ExecuteWithOutput(command string) error {
	var cmd *exec.Cmd
	
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Close 关闭执行器（本地执行器无需关闭）
func (e *LocalExecutor) Close() error {
	return nil
}

// UploadFile 本地执行器不支持文件上传
func (e *LocalExecutor) UploadFile(localPath, remotePath string) error {
	return fmt.Errorf("本地执行器不支持文件上传操作")
}

// CommandExecutor 统一的命令执行接口
type CommandExecutor interface {
	Execute(command string) (string, error)
	Close() error
}

// 确保 LocalExecutor 实现了 CommandExecutor 接口
var _ CommandExecutor = (*LocalExecutor)(nil)

// 确保 SSHClient 实现了 CommandExecutor 接口（已存在）
var _ CommandExecutor = (*SSHClient)(nil)

