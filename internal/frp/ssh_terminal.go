package frp

import (
	"context"
	"fmt"

	"github.com/havline/havline/internal/terminal"
)

// OpenSSHTerminal 用该服务端已保存的 SSH 凭据建立一个交互式 PTY 会话。
// 凭据解密与主机密钥校验（known_hosts TOFU）都复用「安装 Agent」那套 sshInstaller，
// 不再实现一份认证逻辑，也不允许终端页面输入密码。
func (s *Service) OpenSSHTerminal(ctx context.Context, serverID int64, cols, rows int) (terminal.Terminal, string, error) {
	server, installer, err := s.sshInstaller(ctx, serverID)
	if err != nil {
		return nil, "", err
	}
	client, err := installer.client(ctx)
	if err != nil {
		return nil, "", err
	}
	term, err := terminal.NewSSHTerminal(client, cols, rows)
	if err != nil {
		_ = client.Close()
		return nil, "", err
	}
	return term, fmt.Sprintf("%s · %s@%s", server.Name, server.SSHUser, server.SSHHost), nil
}
