package cluster

import (
	_ "embed"
	"fmt"

	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

//go:embed templates/sysctl-k8s.conf
var sysctlConfig string

//go:embed templates/modules-k8s.conf
var modulesConfig string

//go:embed templates/limits-k8s.conf
var limitsConfig string

// OptimizeSystem 优化系统配置（带 UI 输出）
func OptimizeSystem(client *executor.SSHClient) error {
	return optimizeSystemInternal(client, true)
}

// optimizeSystemInternal 优化系统配置的内部实现
func optimizeSystemInternal(client *executor.SSHClient, verbose bool) error {
	if verbose {
		ui.Step(1, 1, "系统优化")
	}
	
	steps := []struct {
		name string
		fn   func() error
	}{
		{"检测操作系统", func() error { return detectOS(client) }},
		{"关闭 swap", func() error { return disableSwap(client) }},
		{"配置性能模式", func() error { return setPerformanceMode(client) }},
		{"关闭防火墙", func() error { return disableFirewall(client) }},
		{"禁用 SELinux", func() error { return disableSELinux(client) }},
		{"配置 sysctl", func() error { return configureSysctl(client) }},
		{"加载内核模块", func() error { return loadKernelModules(client) }},
		{"配置模块自动加载", func() error { return configureModulesAutoload(client) }},
		{"配置系统限制", func() error { return configureSystemLimits(client) }},
		{"配置时间同步", func() error { return configureTimeSync(client) }},
	}
	
	for i, step := range steps {
		if verbose {
			ui.SubStep("[%d/%d] %s...", i+1, len(steps), step.name)
		}
		if err := step.fn(); err != nil {
			if verbose {
				ui.SubStepFailed()
			}
			return fmt.Errorf("%s失败: %w", step.name, err)
		}
		if verbose {
			ui.SubStepDone()
		}
	}
	
	return nil
}

// detectOS 检测操作系统
func detectOS(client *executor.SSHClient) error {
	_, err := client.Execute("cat /etc/os-release")
	return err
}

// disableSwap 关闭 swap
func disableSwap(client *executor.SSHClient) error {
	// 临时关闭
	if _, err := client.Execute("swapoff -a"); err != nil {
		return err
	}
	
	// 永久禁用（注释 fstab 中的 swap 行）
	_, err := client.Execute("sed -i '/swap/s/^/#/' /etc/fstab")
	return err
}

// setPerformanceMode 设置性能模式
func setPerformanceMode(client *executor.SSHClient) error {
	// 检查是否支持 cpupower
	if _, err := client.Execute("which cpupower"); err == nil {
		// 使用 cpupower
		_, err = client.Execute("cpupower frequency-set --governor performance")
		return err
	}
	
	// 直接设置 scaling_governor
	_, err := client.Execute(`
		for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
			if [ -f "$cpu" ]; then
				echo performance > "$cpu" 2>/dev/null || true
			fi
		done
	`)
	return err
}

// disableFirewall 关闭防火墙
func disableFirewall(client *executor.SSHClient) error {
	// 尝试关闭 firewalld
	client.Execute("systemctl stop firewalld 2>/dev/null || true")
	client.Execute("systemctl disable firewalld 2>/dev/null || true")
	
	// 尝试关闭 ufw
	client.Execute("ufw disable 2>/dev/null || true")
	
	return nil
}

// disableSELinux 禁用 SELinux
func disableSELinux(client *executor.SSHClient) error {
	// 临时禁用
	client.Execute("setenforce 0 2>/dev/null || true")
	
	// 永久禁用
	_, err := client.Execute(`
		if [ -f /etc/selinux/config ]; then
			sed -i 's/^SELINUX=enforcing/SELINUX=disabled/' /etc/selinux/config
			sed -i 's/^SELINUX=permissive/SELINUX=disabled/' /etc/selinux/config
		fi
	`)
	return err
}

// configureSysctl 配置 sysctl 参数
func configureSysctl(client *executor.SSHClient) error {
	// 创建临时文件
	tmpFile := "/tmp/99-k8s.conf"
	if _, err := client.Execute(fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", tmpFile, sysctlConfig)); err != nil {
		return err
	}
	
	// 移动到系统目录
	if _, err := client.Execute(fmt.Sprintf("mv %s /etc/sysctl.d/99-k8s.conf", tmpFile)); err != nil {
		return err
	}
	
	// 应用配置
	_, err := client.Execute("sysctl --system")
	return err
}

// loadKernelModules 加载内核模块
func loadKernelModules(client *executor.SSHClient) error {
	modules := []string{
		"overlay",
		"br_netfilter",
		"nf_conntrack",
		// 注意：Cilium eBPF 不需要 IPVS 模块
		// "ip_vs", "ip_vs_rr", "ip_vs_wrr", "ip_vs_sh"
	}
	
	for _, mod := range modules {
		client.Execute(fmt.Sprintf("modprobe %s 2>/dev/null || true", mod))
	}
	
	return nil
}

// configureModulesAutoload 配置模块开机自动加载
func configureModulesAutoload(client *executor.SSHClient) error {
	tmpFile := "/tmp/k8s.conf"
	if _, err := client.Execute(fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", tmpFile, modulesConfig)); err != nil {
		return err
	}
	
	_, err := client.Execute(fmt.Sprintf("mv %s /etc/modules-load.d/k8s.conf", tmpFile))
	return err
}

// configureSystemLimits 配置系统限制
func configureSystemLimits(client *executor.SSHClient) error {
	tmpFile := "/tmp/99-k8s.conf"
	if _, err := client.Execute(fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", tmpFile, limitsConfig)); err != nil {
		return err
	}
	
	_, err := client.Execute(fmt.Sprintf("mv %s /etc/security/limits.d/99-k8s.conf", tmpFile))
	return err
}

// configureTimeSync 配置时间同步
func configureTimeSync(client *executor.SSHClient) error {
	// 检查是否安装了 chrony
	if _, err := client.Execute("which chronyd"); err == nil {
		client.Execute("systemctl enable chronyd")
		client.Execute("systemctl start chronyd")
		return nil
	}
	
	// 检查是否安装了 ntp
	if _, err := client.Execute("which ntpd"); err == nil {
		client.Execute("systemctl enable ntpd")
		client.Execute("systemctl start ntpd")
		return nil
	}
	
	// 如果都没有，尝试安装 chrony
	client.Execute("apt-get install -y chrony 2>/dev/null || yum install -y chrony 2>/dev/null || true")
	
	return nil
}

