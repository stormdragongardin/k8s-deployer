package ui

import (
	"os"

	"github.com/olekukonko/tablewriter"
)

// NewTable 创建一个新的表格
func NewTable(headers []string) *tablewriter.Table {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(headers)
	table.SetBorder(true)
	table.SetRowLine(false)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("|")
	table.SetColumnSeparator("|")
	table.SetRowSeparator("-")
	table.SetHeaderLine(true)
	
	return table
}

// PrintClusterTable 打印集群列表表格
func PrintClusterTable(clusters [][]string) {
	if len(clusters) == 0 {
		Info("没有找到集群")
		return
	}
	
	table := NewTable([]string{"名称", "版本", "Master", "Worker", "GPU", "状态", "创建时间"})
	for _, cluster := range clusters {
		table.Append(cluster)
	}
	table.Render()
}

// PrintNodeTable 打印节点列表表格
func PrintNodeTable(nodes [][]string) {
	if len(nodes) == 0 {
		Info("没有找到节点")
		return
	}
	
	table := NewTable([]string{"主机名", "角色", "IP 地址", "状态", "GPU"})
	for _, node := range nodes {
		table.Append(node)
	}
	table.Render()
}

