package cluster

import (
	_ "embed"
	"bytes"
	"fmt"
	"text/template"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/packages"
	"stormdragon/k8s-deployer/pkg/ui"
)

//go:embed templates/containerd-config.toml
var containerdConfigTemplate string

//go:embed templates/containerd-gpu.toml
var containerdGPUConfigTemplate string

// ContainerdConfig containerd 配置参数
type ContainerdConfig struct {
	ImageRepository string
	HarborHost      string
}

// PrepareNode 准备节点（带 UI 输出）
func PrepareNode(node *config.NodeConfig, imageRepo string, k8sVersion string) error {
	return prepareNodeInternal(node, imageRepo, k8sVersion, true)
}

// PrepareNodeQuiet 准备节点（静默模式，用于并发）
func PrepareNodeQuiet(node *config.NodeConfig, imageRepo string, k8sVersion string) error {
	return prepareNodeInternal(node, imageRepo, k8sVersion, false)
}

// prepareNodeInternal 准备节点的内部实现
func prepareNodeInternal(node *config.NodeConfig, imageRepo string, k8sVersion string, verbose bool) error {
	if verbose {
		ui.Header(fmt.Sprintf("准备节点: %s (%s)", node.Hostname, node.IP))
	}
	
	// 建立 SSH 连接（支持密码或密钥）
	client, err := executor.NewSSHClientWithPassword(
		node.IP, 
		node.SSH.Port, 
		node.SSH.User, 
		node.SSH.KeyFile,
		node.SSH.Password,
	)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()
	
	// 阶段 1: 系统优化
	if err := optimizeSystemInternal(client, verbose); err != nil {
		return err
	}
	
	// 阶段 2: 安装容器运行时
	if verbose {
		ui.Step(2, 4, "安装容器运行时 (containerd)")
	}
	if err := installContainerd(client, imageRepo, node.GPU); err != nil {
		return err
	}
	
	// 阶段 3: 安装 Kubernetes 组件
	if verbose {
		ui.Step(3, 4, "安装 Kubernetes 组件")
	}
	if err := installK8sComponents(client, k8sVersion); err != nil {
		return err
	}
	
	// 阶段 4: GPU 节点特殊处理
	if node.GPU {
		if verbose {
			ui.Step(4, 4, "配置 GPU 支持")
		}
		if err := configureGPU(client); err != nil {
			return err
		}
	}
	
	if verbose {
		ui.Success("节点 %s 准备完成！", node.Hostname)
	}
	return nil
}

// installContainerd 安装 containerd（使用离线包）
func installContainerd(client *executor.SSHClient, imageRepo string, isGPU bool) error {
	// 初始化包管理器
	pkgMgr := packages.NewManager()
	
	// 检查本地离线包
	ui.SubStep("检查离线包...")
	requiredPkgs := []string{"containerd", "runc", "cni-plugins"}
	missingPkgs := pkgMgr.CheckRequiredPackages(requiredPkgs)
	if len(missingPkgs) > 0 {
		ui.SubStepFailed()
		return fmt.Errorf("缺少离线包，请先运行: cd scripts && ./download-all.sh")
	}
	ui.SubStepDone()
	
	// 停止旧的 containerd 服务（如果存在）
	ui.SubStep("停止旧的 containerd 服务...")
	client.Execute("systemctl stop containerd")
	ui.SubStepDone()
	
	// 上传并安装 containerd 二进制包（强制覆盖）
	ui.SubStep("安装 containerd...")
	containerdTar := pkgMgr.GetPackagePath("containerd")
	if err := client.UploadFile(containerdTar, "/tmp/containerd.tar.gz"); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 containerd 失败: %w", err)
	}
	
	// 解压并安装 containerd（覆盖旧文件）
	installCmd := `
		cd /tmp
		tar -xzf containerd.tar.gz -C /usr/local
		rm -f containerd.tar.gz
		
		# 创建 systemd 服务（覆盖）
		cat > /etc/systemd/system/containerd.service << 'EOF'
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/containerd
Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=infinity
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
EOF
	`
	if _, err := client.Execute(installCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("安装 containerd 失败: %w", err)
	}
	ui.SubStepDone()
	
	// 安装 runc（强制覆盖）
	ui.SubStep("安装 runc...")
	runcPath := pkgMgr.GetPackagePath("runc")
	if err := client.UploadFile(runcPath, "/tmp/runc.amd64"); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 runc 失败: %w", err)
	}
	
	runcInstallCmd := `
		install -m 755 /tmp/runc.amd64 /usr/local/sbin/runc
		rm -f /tmp/runc.amd64
	`
	if _, err := client.Execute(runcInstallCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("安装 runc 失败: %w", err)
	}
	ui.SubStepDone()
	
	// 安装 CNI plugins（强制覆盖）
	ui.SubStep("安装 CNI plugins...")
	cniPath := pkgMgr.GetPackagePath("cni-plugins")
	if err := client.UploadFile(cniPath, "/tmp/cni-plugins.tgz"); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 CNI plugins 失败: %w", err)
	}
	
	cniInstallCmd := `
		mkdir -p /opt/cni/bin
		tar -xzf /tmp/cni-plugins.tgz -C /opt/cni/bin
		rm -f /tmp/cni-plugins.tgz
	`
	if _, err := client.Execute(cniInstallCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("安装 CNI plugins 失败: %w", err)
	}
	ui.SubStepDone()
	
	// 配置 containerd（强制覆盖配置文件）
	ui.SubStep("配置 containerd...")
	if err := configureContainerd(client, imageRepo, isGPU); err != nil {
		return err
	}
	
	// 启动 containerd
	ui.SubStep("启动 containerd...")
	startCmd := `
		# 创建符号链接以兼容旧路径
		mkdir -p /var/run/containerd
		ln -sf /run/containerd/containerd.sock /var/run/containerd/containerd.sock
		
		systemctl daemon-reload
		systemctl enable containerd
		systemctl restart containerd
	`
	if _, err := client.Execute(startCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("启动 containerd 失败: %w", err)
	}
	ui.SubStepDone()
	
	return nil
}

// configureContainerd 配置 containerd
func configureContainerd(client *executor.SSHClient, imageRepo string, isGPU bool) error {
	return generateContainerdConfig(client, imageRepo, isGPU)
}

// generateContainerdConfig 生成 containerd 配置
func generateContainerdConfig(client *executor.SSHClient, imageRepo string, isGPU bool) error {
	// 解析 Harbor 主机
	harborHost := imageRepo
	if len(harborHost) > 7 && harborHost[:7] == "http://" {
		harborHost = harborHost[7:]
	} else if len(harborHost) > 8 && harborHost[:8] == "https://" {
		harborHost = harborHost[8:]
	}
	// 移除路径部分
	if idx := bytes.IndexByte([]byte(harborHost), '/'); idx != -1 {
		harborHost = harborHost[:idx]
	}
	
	params := ContainerdConfig{
		ImageRepository: imageRepo,
		HarborHost:      harborHost,
	}
	
	// 选择模板
	templateStr := containerdConfigTemplate
	if isGPU {
		templateStr = containerdGPUConfigTemplate
	}
	
	// 渲染模板
	tmpl, err := template.New("containerd").Parse(templateStr)
	if err != nil {
		return err
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return err
	}
	
	// 写入配置文件
	tmpFile := "/tmp/containerd-config.toml"
	configContent := buf.String()
	
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", tmpFile, configContent)
	if _, err := client.Execute(cmd); err != nil {
		return err
	}
	
	// 创建目录并移动配置
	_, err = client.Execute(`
		mkdir -p /etc/containerd
		mv /tmp/containerd-config.toml /etc/containerd/config.toml
	`)
	if err != nil {
		return err
	}
	
	// 创建镜像仓库配置目录和 hosts.toml
	// 使用 config_path 方式配置镜像仓库（兼容 containerd v2.x）
	hostsTomlContent := fmt.Sprintf(`server = "http://%s"

[host."http://%s"]
  capabilities = ["pull", "resolve", "push"]
  skip_verify = true
`, harborHost, harborHost)
	
	hostsCmd := fmt.Sprintf("cat > /tmp/hosts.toml << 'EOF'\n%s\nEOF", hostsTomlContent)
	if _, err := client.Execute(hostsCmd); err != nil {
		return err
	}
	
	_, err = client.Execute(fmt.Sprintf(`
		mkdir -p /etc/containerd/certs.d/%s
		mv /tmp/hosts.toml /etc/containerd/certs.d/%s/hosts.toml
	`, harborHost, harborHost))
	
	return err
}

// installK8sComponents 安装 Kubernetes 组件（使用离线包）
func installK8sComponents(client *executor.SSHClient, k8sVersion string) error {
	// 初始化包管理器（使用指定的 K8s 版本）
	pkgMgr := packages.NewManagerWithVersion(k8sVersion)
	
	// 检查本地离线包
	ui.SubStep("检查 K8s 离线包...")
	requiredPkgs := []string{"kubectl", "kubeadm", "kubelet"}
	missingPkgs := pkgMgr.CheckRequiredPackages(requiredPkgs)
	if len(missingPkgs) > 0 {
		ui.SubStepFailed()
		return fmt.Errorf("缺少离线包，请先运行: cd scripts && ./download-all.sh")
	}
	ui.SubStepDone()
	
	// 上传 kubectl
	ui.SubStep("上传 kubectl...")
	kubectlBin := pkgMgr.GetPackagePath("kubectl")
	if err := client.UploadFile(kubectlBin, "/tmp/kubectl"); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 kubectl 失败: %w", err)
	}
	ui.SubStepDone()
	
	// 上传 kubeadm
	ui.SubStep("上传 kubeadm...")
	kubeadmBin := pkgMgr.GetPackagePath("kubeadm")
	if err := client.UploadFile(kubeadmBin, "/tmp/kubeadm"); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 kubeadm 失败: %w", err)
	}
	ui.SubStepDone()
	
	// 上传 kubelet
	ui.SubStep("上传 kubelet...")
	kubeletBin := pkgMgr.GetPackagePath("kubelet")
	if err := client.UploadFile(kubeletBin, "/tmp/kubelet"); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("上传 kubelet 失败: %w", err)
	}
	ui.SubStepDone()
	
	// 安装二进制文件
	ui.SubStep("安装 K8s 组件...")
	installCmd := `
		install -m 755 /tmp/kubectl /usr/local/bin/kubectl
		install -m 755 /tmp/kubeadm /usr/local/bin/kubeadm
		install -m 755 /tmp/kubelet /usr/local/bin/kubelet
		rm -f /tmp/kubectl /tmp/kubeadm /tmp/kubelet
		
		# 创建 kubelet systemd 服务
		mkdir -p /etc/systemd/system/kubelet.service.d
		
		cat > /etc/systemd/system/kubelet.service << 'EOF'
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/local/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

		cat > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf << 'EOF'
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
Environment="KUBELET_EXTRA_ARGS=--container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
EOF

		systemctl daemon-reload
		systemctl enable kubelet
	`
	
	if _, err := client.Execute(installCmd); err != nil {
		ui.SubStepFailed()
		return fmt.Errorf("安装 K8s 组件失败: %w", err)
	}
	ui.SubStepDone()
	
	return nil
}
