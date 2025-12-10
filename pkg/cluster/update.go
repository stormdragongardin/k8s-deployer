package cluster

import (
	"fmt"

	"stormdragon/k8s-deployer/pkg/config"
	"stormdragon/k8s-deployer/pkg/executor"
	"stormdragon/k8s-deployer/pkg/ui"
)

// ConfigChange 配置变更
type ConfigChange struct {
	Type              string // 变更类型
	Description       string // 变更描述
	OldValue          string // 旧值
	NewValue          string // 新值
	AffectedComponent string // 受影响的组件
	RequiresRestart   bool   // 是否需要重启
}

// UpdateCluster 更新集群配置（使用本地 kubectl）
func UpdateCluster(newCfg *config.ClusterConfig, onlyBGP bool, autoConfirm bool) error {
	ui.Info("集群名称: %s", newCfg.Metadata.Name)

	// 使用本地执行器
	ui.Info("使用本地 kubectl 连接集群...")
	client := executor.NewLocalExecutor()

	// 验证集群存在（本地 kubectl）
	if err := verifyClusterExistsLocal(client); err != nil {
		return fmt.Errorf("集群验证失败: %w，请确保本地 kubectl 已正确配置", err)
	}
	ui.Success("集群连接成功")

	// 获取当前集群配置
	ui.Info("加载当前集群配置...")
	oldCfg, err := LoadClusterConfigLocal(client, newCfg.Metadata.Name)
	if err != nil {
		ui.Warning("加载集群配置失败: %v，将跳过不可变字段检查", err)
		oldCfg = nil
	} else {
		ui.Success("当前配置加载成功")
	}

	// 验证不可变字段
	if oldCfg != nil {
		ui.Info("检查不可变配置...")
		if err := config.ValidateImmutableFields(oldCfg, newCfg); err != nil {
			ui.Error("配置验证失败:")
			ui.Error("%v", err)
			ui.Info("")
			ui.Info("不可变配置包括:")
			ui.Info("  - 集群名称 (metadata.name)")
			ui.Info("  - Pod 网段 (spec.networking.podSubnet)")
			ui.Info("  - Service 网段 (spec.networking.serviceSubnet)")
			ui.Info("  - Kubernetes 版本 (spec.version)")
			return fmt.Errorf("配置验证失败")
		}
		ui.Success("不可变配置检查通过")
	}

	// 检测并显示变更
	ui.Header("检测配置变更")
	var changes []ConfigChange

	if onlyBGP {
		changes = detectBGPChanges(oldCfg, newCfg)
	} else {
		changes = detectAllChanges(oldCfg, newCfg)
	}

	if len(changes) == 0 {
		ui.Info("未检测到配置变更")
		return nil
	}

	// 显示变更详情
	displayChanges(changes)

	// 确认变更（除非使用 --yes 标志）
	if !autoConfirm {
		ui.Info("")
		ui.Warning("以上操作将会:")
		for _, change := range changes {
			if change.RequiresRestart {
				ui.Warning("  - %s (需要重启 %s)", change.Description, change.AffectedComponent)
			} else {
				ui.Info("  - %s", change.Description)
			}
		}
		ui.Info("")

		if !ui.WaitForConfirmation("确认执行以上变更？") {
			ui.Warning("更新已取消")
			return nil
		}
	}

	// 执行更新
	var updateErr error
	if onlyBGP {
		updateErr = updateBGPOnly(client, newCfg)
	} else {
		updateErr = updateFull(client, oldCfg, newCfg)
	}

	if updateErr != nil {
		return updateErr
	}

	// 更新成功后，保存新配置到 ConfigMap
	ui.Info("")
	ui.Info("更新集群配置记录...")
	if err := UpdateClusterConfigMap(client, newCfg); err != nil {
		ui.Warning("更新配置记录失败: %v", err)
		ui.Warning("这不影响集群使用，但配置记录可能不同步")
	} else {
		ui.Success("配置记录已更新")
	}

	return nil
}

// detectBGPChanges 检测 BGP 配置变更
func detectBGPChanges(oldCfg, newCfg *config.ClusterConfig) []ConfigChange {
	var changes []ConfigChange

	// 如果没有旧配置，认为是首次配置 BGP
	if oldCfg == nil {
		if newCfg.Spec.BGP.Enabled {
			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       "启用 BGP 控制平面",
				OldValue:          "未配置",
				NewValue:          "启用",
				AffectedComponent: "Cilium",
				RequiresRestart:   true,
			})

			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       fmt.Sprintf("配置 BGP AS 号: %d", newCfg.Spec.BGP.LocalASN),
				NewValue:          fmt.Sprintf("%d", newCfg.Spec.BGP.LocalASN),
				AffectedComponent: "Cilium",
				RequiresRestart:   false,
			})

			for i, peer := range newCfg.Spec.BGP.Peers {
				changes = append(changes, ConfigChange{
					Type:              "BGP",
					Description:       fmt.Sprintf("添加 BGP Peer %d: %s (AS %d)", i+1, peer.PeerAddress, peer.PeerASN),
					NewValue:          fmt.Sprintf("%s/%d", peer.PeerAddress, peer.PeerASN),
					AffectedComponent: "BGP Peering",
					RequiresRestart:   false,
				})
			}

			for i, ip := range newCfg.Spec.BGP.LoadBalancerIPs {
				changes = append(changes, ConfigChange{
					Type:              "BGP",
					Description:       fmt.Sprintf("添加 LoadBalancer IP 池 %d: %s", i+1, ip),
					NewValue:          ip,
					AffectedComponent: "IP Pool",
					RequiresRestart:   false,
				})
			}
		}
		return changes
	}

	// BGP 启用状态变更
	if !oldCfg.Spec.BGP.Enabled && newCfg.Spec.BGP.Enabled {
		changes = append(changes, ConfigChange{
			Type:              "BGP",
			Description:       "启用 BGP 控制平面",
			OldValue:          "禁用",
			NewValue:          "启用",
			AffectedComponent: "Cilium",
			RequiresRestart:   true,
		})

		changes = append(changes, ConfigChange{
			Type:              "BGP",
			Description:       fmt.Sprintf("配置 BGP AS 号: %d", newCfg.Spec.BGP.LocalASN),
			NewValue:          fmt.Sprintf("%d", newCfg.Spec.BGP.LocalASN),
			AffectedComponent: "Cilium",
			RequiresRestart:   false,
		})

		for i, peer := range newCfg.Spec.BGP.Peers {
			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       fmt.Sprintf("添加 BGP Peer %d: %s (AS %d)", i+1, peer.PeerAddress, peer.PeerASN),
				NewValue:          fmt.Sprintf("%s/%d", peer.PeerAddress, peer.PeerASN),
				AffectedComponent: "BGP Peering",
				RequiresRestart:   false,
			})
		}

		for i, ip := range newCfg.Spec.BGP.LoadBalancerIPs {
			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       fmt.Sprintf("添加 LoadBalancer IP 池 %d: %s", i+1, ip),
				NewValue:          ip,
				AffectedComponent: "IP Pool",
				RequiresRestart:   false,
			})
		}
	} else if oldCfg.Spec.BGP.Enabled && newCfg.Spec.BGP.Enabled {
		// BGP 已启用，检测配置变更
		if oldCfg.Spec.BGP.LocalASN != newCfg.Spec.BGP.LocalASN {
			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       "修改 BGP AS 号",
				OldValue:          fmt.Sprintf("%d", oldCfg.Spec.BGP.LocalASN),
				NewValue:          fmt.Sprintf("%d", newCfg.Spec.BGP.LocalASN),
				AffectedComponent: "BGP Peering",
				RequiresRestart:   false,
			})
		}

		// 检测 Peer 变更（简化实现）
		if len(oldCfg.Spec.BGP.Peers) != len(newCfg.Spec.BGP.Peers) {
			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       "更新 BGP Peer 配置",
				OldValue:          fmt.Sprintf("%d 个 Peer", len(oldCfg.Spec.BGP.Peers)),
				NewValue:          fmt.Sprintf("%d 个 Peer", len(newCfg.Spec.BGP.Peers)),
				AffectedComponent: "BGP Peering",
				RequiresRestart:   false,
			})
		}

		// 检测 IP 池变更
		if len(oldCfg.Spec.BGP.LoadBalancerIPs) != len(newCfg.Spec.BGP.LoadBalancerIPs) {
			changes = append(changes, ConfigChange{
				Type:              "BGP",
				Description:       "更新 LoadBalancer IP 池",
				OldValue:          fmt.Sprintf("%d 个 IP", len(oldCfg.Spec.BGP.LoadBalancerIPs)),
				NewValue:          fmt.Sprintf("%d 个 IP", len(newCfg.Spec.BGP.LoadBalancerIPs)),
				AffectedComponent: "IP Pool",
				RequiresRestart:   false,
			})
		}
	}

	return changes
}

// detectAllChanges 检测所有配置变更
func detectAllChanges(oldCfg, newCfg *config.ClusterConfig) []ConfigChange {
	var changes []ConfigChange

	// 如果没有旧配置，跳过检测
	if oldCfg == nil {
		ui.Warning("由于无法加载旧配置，跳过变更检测")
		return changes
	}

	// BGP 变更
	bgpChanges := detectBGPChanges(oldCfg, newCfg)
	changes = append(changes, bgpChanges...)

	// Harbor 认证变更
	if oldCfg.Spec.Harbor.Username != newCfg.Spec.Harbor.Username ||
		oldCfg.Spec.Harbor.Password != newCfg.Spec.Harbor.Password {
		changes = append(changes, ConfigChange{
			Type:              "Harbor",
			Description:       "更新 Harbor 认证信息",
			AffectedComponent: "Containerd",
			RequiresRestart:   false,
		})
	}

	return changes
}

// displayChanges 显示变更详情
func displayChanges(changes []ConfigChange) {
	ui.Info("检测到 %d 项配置变更:", len(changes))
	ui.Info("")

	for i, change := range changes {
		ui.Info("[变更 %d/%d] %s", i+1, len(changes), change.Type)
		ui.Info("  描述: %s", change.Description)

		if change.OldValue != "" {
			ui.Info("  当前值: %s", change.OldValue)
		}
		if change.NewValue != "" {
			ui.Info("  新值: %s", change.NewValue)
		}

		ui.Info("  影响组件: %s", change.AffectedComponent)

		if change.RequiresRestart {
			ui.Warning("  ⚠️  需要重启 %s", change.AffectedComponent)
		}

		ui.Info("")
	}
}

// updateBGPOnly 仅更新 BGP 配置
func updateBGPOnly(client executor.CommandExecutor, cfg *config.ClusterConfig) error {
	ui.Header("更新 BGP 配置")

	if !cfg.Spec.BGP.Enabled {
		return fmt.Errorf("配置中未启用 BGP，无法更新")
	}

	// 1. 检查当前 BGP 状态
	ui.Step(1, 3, "检查当前 BGP 状态")
	bgpEnabled, err := checkBGPEnabled(client)
	if err != nil {
		return err
	}

	if bgpEnabled {
		ui.Info("BGP 已启用，将更新现有配置")
	} else {
		ui.Info("BGP 未启用，将首次启用 BGP")
	}

	// 2. 安装/更新 MetalLB
	ui.Step(2, 3, "安装/更新 MetalLB")
	if err := InstallMetalLB(client, cfg); err != nil {
		return err
	}

	ui.Success("BGP 配置更新完成！")
	ui.Info("")
	ui.Info("验证 MetalLB BGP 状态:")
	ui.Info("  kubectl get ipaddresspool -n metallb-system")
	ui.Info("  kubectl get bgppeer -n metallb-system")
	ui.Info("  kubectl get bgpadvertisement -n metallb-system")
	ui.Info("  kubectl get svc -A | grep LoadBalancer")

	return nil
}

// upgradeCiliumForBGP 已废弃 - BGP 现在由 MetalLB 提供
func upgradeCiliumForBGP(client executor.CommandExecutor, _ *config.ClusterConfig) error {
	// 此函数保留以兼容性，但不再使用
	return nil
}

// checkBGPEnabled 检查 BGP 是否已启用（检查 MetalLB）
func checkBGPEnabled(client executor.CommandExecutor) (bool, error) {
	_, err := client.Execute("kubectl get bgppeer -n metallb-system 2>/dev/null")
	return err == nil, nil
}

// waitForCilium 等待 Cilium 就绪
func waitForCilium(client executor.CommandExecutor) error {
	cmd := `kubectl rollout status daemonset/cilium -n kube-system --timeout=300s`
	_, err := client.Execute(cmd)
	return err
}

// verifyClusterExistsLocal 验证集群是否存在（本地 kubectl）
func verifyClusterExistsLocal(client *executor.LocalExecutor) error {
	_, err := client.Execute("kubectl cluster-info")
	return err
}

// LoadClusterConfigLocal 从集群加载配置（使用本地 kubectl）
func LoadClusterConfigLocal(client *executor.LocalExecutor, clusterName string) (*config.ClusterConfig, error) {
	// 直接调用 LoadClusterConfig，传入接口类型
	return LoadClusterConfig(client, clusterName)
}

// updateFull 完整更新
func updateFull(client executor.CommandExecutor, oldCfg, newCfg *config.ClusterConfig) error {
	ui.Header("应用配置变更")

	changes := detectAllChanges(oldCfg, newCfg)

	if len(changes) == 0 {
		ui.Info("未检测到可更新的配置变更")
		return nil
	}

	// 应用变更
	for _, change := range changes {
		switch change.Type {
		case "BGP":
			if err := updateBGPOnly(client, newCfg); err != nil {
				return err
			}
		}
	}

	ui.Success("配置更新完成！")
	return nil
}
