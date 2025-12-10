package ui

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/schollz/progressbar/v3"
)

// NewSpinner 创建一个新的 spinner
func NewSpinner(message string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + message
	return s
}

// StartSpinner 启动 spinner 并返回停止函数
func StartSpinner(message string) func(bool) {
	s := NewSpinner(message)
	s.Start()
	
	return func(success bool) {
		s.Stop()
		if success {
			Success(message)
		} else {
			Error(message + " 失败")
		}
	}
}

// NewProgressBar 创建新的进度条
func NewProgressBar(max int, description string) *progressbar.ProgressBar {
	return progressbar.NewOptions(max,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWidth(50),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
}

// ProgressStep 进度步骤
type ProgressStep struct {
	Name string
	Done bool
}

// ShowProgressSteps 显示进度步骤列表
func ShowProgressSteps(steps []ProgressStep) {
	fmt.Println()
	for i, step := range steps {
		if step.Done {
			ColorSuccess.Printf("  [%d/%d] %s ✓\n", i+1, len(steps), step.Name)
		} else {
			fmt.Printf("  [%d/%d] %s\n", i+1, len(steps), step.Name)
		}
	}
	fmt.Println()
}

