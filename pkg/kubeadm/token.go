package kubeadm

import (
	"fmt"
	"regexp"
	"strings"

	"stormdragon/k8s-deployer/pkg/executor"
)

// TokenInfo Token 信息
type TokenInfo struct {
	Token          string
	TTL            string
	Expires        string
	Usages         string
	Description    string
	CertificateKey string
}

// CreateToken 创建新的 bootstrap token
func CreateToken(client *executor.SSHClient, ttl string) (string, error) {
	cmd := fmt.Sprintf("kubeadm token create --ttl %s", ttl)
	output, err := client.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("创建 token 失败: %w", err)
	}
	
	// 提取 token（去除换行符）
	token := strings.TrimSpace(output)
	return token, nil
}

// ListTokens 列出所有 token
func ListTokens(client *executor.SSHClient) ([]TokenInfo, error) {
	output, err := client.Execute("kubeadm token list")
	if err != nil {
		return nil, fmt.Errorf("列出 token 失败: %w", err)
	}
	
	// 解析输出
	lines := strings.Split(output, "\n")
	var tokens []TokenInfo
	
	// 跳过表头
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			tokens = append(tokens, TokenInfo{
				Token:   fields[0],
				TTL:     fields[1],
				Expires: fields[2],
				Usages:  fields[3],
			})
		}
	}
	
	return tokens, nil
}

// DeleteToken 删除指定的 token
func DeleteToken(client *executor.SSHClient, token string) error {
	cmd := fmt.Sprintf("kubeadm token delete %s", token)
	_, err := client.Execute(cmd)
	if err != nil {
		return fmt.Errorf("删除 token 失败: %w", err)
	}
	return nil
}

// GetCACertHash 获取 CA 证书哈希
func GetCACertHash(client *executor.SSHClient) (string, error) {
	cmd := `openssl x509 -pubkey -in /etc/kubernetes/pki/ca.crt | \
		openssl rsa -pubin -outform der 2>/dev/null | \
		openssl dgst -sha256 -hex | sed 's/^.* //'`
	
	output, err := client.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("获取 CA 证书哈希失败: %w", err)
	}
	
	hash := strings.TrimSpace(output)
	return "sha256:" + hash, nil
}

// UploadCerts 上传证书并返回证书密钥
func UploadCerts(client *executor.SSHClient) (string, error) {
	output, err := client.Execute("kubeadm init phase upload-certs --upload-certs")
	if err != nil {
		return "", fmt.Errorf("上传证书失败: %w", err)
	}
	
	// 从输出中提取证书密钥
	// 输出格式类似: [upload-certs] Using certificate key: abc123...
	re := regexp.MustCompile(`certificate key:\s+([a-f0-9]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return "", fmt.Errorf("无法从输出中提取证书密钥")
	}
	
	return matches[1], nil
}

// GetJoinInfo 获取 join 所需的所有信息
func GetJoinInfo(client *executor.SSHClient, controlPlaneEndpoint string, forMaster bool) (*JoinCommand, error) {
	// 创建 token (24小时有效)
	token, err := CreateToken(client, "24h")
	if err != nil {
		return nil, err
	}
	
	// 获取 CA 证书哈希
	caCertHash, err := GetCACertHash(client)
	if err != nil {
		return nil, err
	}
	
	joinCmd := &JoinCommand{
		APIServerEndpoint: controlPlaneEndpoint,
		Token:             token,
		CACertHash:        caCertHash,
	}
	
	// 如果是 master 节点，需要上传证书
	if forMaster {
		certKey, err := UploadCerts(client)
		if err != nil {
			return nil, err
		}
		joinCmd.CertificateKey = certKey
	}
	
	return joinCmd, nil
}

