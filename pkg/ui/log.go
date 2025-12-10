package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

var (
	// 颜色定义
	ColorInfo    = color.New(color.FgCyan)
	ColorSuccess = color.New(color.FgGreen)
	ColorWarning = color.New(color.FgYellow)
	ColorError   = color.New(color.FgRed)
	ColorBold    = color.New(color.Bold)
)

// Info 打印信息消息
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ColorInfo.Printf("[信息] %s\n", msg)
}

// Success 打印成功消息
func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ColorSuccess.Printf("✓ %s\n", msg)
}

// Warning 打印警告消息
func Warning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ColorWarning.Printf("[警告] %s\n", msg)
}

// Warn 打印警告消息（Warning 的别名）
func Warn(format string, args ...interface{}) {
	Warning(format, args...)
}

// Error 打印错误消息
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ColorError.Fprintf(os.Stderr, "✗ 错误: %s\n", msg)
}

// Confirm 询问用户确认（WaitForConfirmation 的别名）
func Confirm(message string) bool {
	return WaitForConfirmation(message)
}

// Step 打印步骤信息
func Step(current, total int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ColorBold.Printf("\n[%d/%d] %s\n", current, total, msg)
}

// SubStep 打印子步骤信息
func SubStep(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("  → %s", msg)
}

// SubStepDone 子步骤完成
func SubStepDone() {
	ColorSuccess.Println(" ✓")
}

// SubStepFailed 子步骤失败
func SubStepFailed() {
	ColorError.Println(" ✗")
}

// Title 打印标题
func Title(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println()
	ColorBold.Println(msg)
	fmt.Println(strings.Repeat("=", len(msg)))
}

// Divider 打印分隔线
func Divider() {
	fmt.Println(strings.Repeat("-", 60))
}

// Header 打印大标题
func Header(text string) {
	width := 60
	fmt.Println()
	fmt.Println(strings.Repeat("=", width))
	padding := (width - len(text)) / 2
	fmt.Printf("%s%s\n", strings.Repeat(" ", padding), text)
	fmt.Println(strings.Repeat("=", width))
	fmt.Println()
}

// WaitForConfirmation 等待用户确认（默认为是）
func WaitForConfirmation(message string) bool {
	fmt.Printf("%s [Y/n]: ", message)
	var response string
	fmt.Scanln(&response)
	// 空输入（直接回车）默认为 yes
	if response == "" {
		return true
	}
	return response == "y" || response == "Y" || response == "yes" || response == "Yes"
}

// WaitForDangerousConfirmation 等待用户确认危险操作（必须明确输入yes）
func WaitForDangerousConfirmation(message string) bool {
	ColorWarning.Printf("⚠️  %s\n", message)
	ColorWarning.Printf("请输入 'yes' 确认操作: ")
	var response string
	fmt.Scanln(&response)
	return response == "yes"
}

// PrintClusterInfo 打印集群信息
func PrintClusterInfo(name, version string, masters, workers, gpuNodes int) {
	Header("集群信息")
	fmt.Printf("  名称: %s\n", name)
	fmt.Printf("  版本: %s\n", version)
	fmt.Printf("  Master 节点: %d\n", masters)
	fmt.Printf("  Worker 节点: %d\n", workers)
	if gpuNodes > 0 {
		fmt.Printf("  GPU 节点: %d\n", gpuNodes)
	}
	Divider()
}

