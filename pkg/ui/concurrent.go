package ui

import (
	"fmt"
	"sync"
	"time"
)

// NodeProgress 节点进度跟踪
type NodeProgress struct {
	Name    string
	Status  string // preparing, success, failed
	Message string
	mu      sync.Mutex
}

// ConcurrentProgressTracker 并发进度跟踪器
type ConcurrentProgressTracker struct {
	nodes    []*NodeProgress
	mu       sync.Mutex
	startRow int
}

// NewConcurrentProgressTracker 创建并发进度跟踪器
func NewConcurrentProgressTracker(nodeNames []string) *ConcurrentProgressTracker {
	tracker := &ConcurrentProgressTracker{
		nodes: make([]*NodeProgress, len(nodeNames)),
	}
	
	for i, name := range nodeNames {
		tracker.nodes[i] = &NodeProgress{
			Name:    name,
			Status:  "pending",
			Message: "等待中...",
		}
	}
	
	return tracker
}

// Start 开始显示进度
func (t *ConcurrentProgressTracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// 打印初始状态
	fmt.Println()
	for i, node := range t.nodes {
		fmt.Printf("[%d/%d] %-20s | ⏳ %s\n", i+1, len(t.nodes), node.Name, node.Message)
	}
	
	// 移动光标到开始位置（为后续更新做准备）
	// 保存当前行号
	t.startRow = len(t.nodes)
}

// UpdateNode 更新节点状态
func (t *ConcurrentProgressTracker) UpdateNode(nodeName, status, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// 找到节点
	var nodeIdx int
	var node *NodeProgress
	for i, n := range t.nodes {
		if n.Name == nodeName {
			node = n
			nodeIdx = i
			break
		}
	}
	
	if node == nil {
		return
	}
	
	// 更新状态
	node.mu.Lock()
	node.Status = status
	node.Message = message
	node.mu.Unlock()
	
	// 重新打印所有节点（简单实现）
	// 在实际终端中，可以使用 ANSI 转义码更新特定行
	t.redrawAll()
	
	// 如果是最终状态，打印单独的完成消息
	if status == "success" {
		fmt.Printf("\n✓ [%d/%d] %s 完成\n", nodeIdx+1, len(t.nodes), nodeName)
	} else if status == "failed" {
		fmt.Printf("\n✗ [%d/%d] %s 失败: %s\n", nodeIdx+1, len(t.nodes), nodeName, message)
	}
}

// redrawAll 重新绘制所有节点（简化版本，实际可以用 ANSI 码优化）
func (t *ConcurrentProgressTracker) redrawAll() {
	// 简单版本：只打印状态变化
	// 完整版本可以使用 github.com/buger/goterm 或类似库
}

// Finish 完成所有进度显示
func (t *ConcurrentProgressTracker) Finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	fmt.Println()
	
	// 统计结果
	success := 0
	failed := 0
	for _, node := range t.nodes {
		if node.Status == "success" {
			success++
		} else if node.Status == "failed" {
			failed++
		}
	}
	
	fmt.Printf("\n")
	fmt.Printf("========================================\n")
	fmt.Printf("并发操作完成: ✓ %d 成功, ✗ %d 失败\n", success, failed)
	fmt.Printf("========================================\n")
	fmt.Println()
}

// SimpleProgressLogger 简化的进度日志（不需要复杂的终端控制）
type SimpleProgressLogger struct {
	nodePrefix map[string]string
	mu         sync.Mutex
}

// NewSimpleProgressLogger 创建简化进度日志
func NewSimpleProgressLogger(nodeNames []string) *SimpleProgressLogger {
	logger := &SimpleProgressLogger{
		nodePrefix: make(map[string]string),
	}
	
	// 为每个节点分配颜色前缀
	colors := []string{
		"\033[36m", // 青色
		"\033[33m", // 黄色
		"\033[32m", // 绿色
		"\033[35m", // 紫色
		"\033[34m", // 蓝色
		"\033[31m", // 红色
		"\033[37m", // 白色
		"\033[90m", // 灰色
	}
	
	for i, name := range nodeNames {
		color := colors[i%len(colors)]
		logger.nodePrefix[name] = color
	}
	
	return logger
}

// Log 记录节点日志
func (l *SimpleProgressLogger) Log(nodeName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	color := l.nodePrefix[nodeName]
	reset := "\033[0m"
	timestamp := time.Now().Format("15:04:05")
	
	fmt.Printf("%s[%s] %-20s%s | %s\n", color, timestamp, nodeName, reset, message)
}

// Success 记录成功
func (l *SimpleProgressLogger) Success(nodeName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	green := "\033[32m"
	reset := "\033[0m"
	timestamp := time.Now().Format("15:04:05")
	
	fmt.Printf("%s[%s] %-20s%s | ✓ %s\n", green, timestamp, nodeName, reset, message)
}

// Error 记录错误
func (l *SimpleProgressLogger) Error(nodeName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	red := "\033[31m"
	reset := "\033[0m"
	timestamp := time.Now().Format("15:04:05")
	
	fmt.Printf("%s[%s] %-20s%s | ✗ %s\n", red, timestamp, nodeName, reset, message)
}

