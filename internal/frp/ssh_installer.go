package frp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHInstaller 通过固定白名单命令在 VPS 上安装/升级 havline-agent。
// agent 的日常管理仍走 AgentClient；SSH 只承担生命周期和失联自救。
type SSHInstaller struct {
	Host       string
	Port       int
	User       string
	Password   string
	PrivateKey string
	Timeout    time.Duration
	// KnownHostsPath 是主机密钥记录文件：首次连接按 TOFU 记录指纹，之后必须一致
	KnownHostsPath string
}

// hostKeyCallback 用 known_hosts 校验主机密钥：首次见到的主机按 TOFU 记录指纹，
// 已记录但与当前不一致则拒绝连接（避免中间人截获 VPS 的 root 凭据）。
func (i SSHInstaller) hostKeyCallback() (ssh.HostKeyCallback, error) {
	path := strings.TrimSpace(i.KnownHostsPath)
	if path == "" {
		return nil, fmt.Errorf("未配置 SSH 主机密钥记录文件")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return nil, err
		}
	}
	known, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("读取 SSH 主机密钥记录失败: %w", err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := known(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				return appendKnownHost(path, remote, key)
			}
			return fmt.Errorf("SSH 主机密钥与首次记录不一致（%s 期望 %s，实际 %s）；若确认 VPS 重装过系统，请删除 %s 后重连",
				remote.String(), ssh.FingerprintSHA256(keyErr.Want[0].Key), ssh.FingerprintSHA256(key), path)
		}
		return fmt.Errorf("SSH 主机密钥校验失败: %w", err)
	}, nil
}

// appendKnownHost 把首次见到的主机密钥追加进 known_hosts（TOFU：首次信任，之后必须一致）。
func appendKnownHost(path string, remote net.Addr, key ssh.PublicKey) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	line := knownhosts.Line([]string{knownhosts.Normalize(remote.String())}, key)
	if _, err := file.WriteString(line + "\n"); err != nil {
		return err
	}
	return nil
}

func (i SSHInstaller) client(ctx context.Context) (*ssh.Client, error) {
	if i.Port == 0 {
		i.Port = 22
	}
	var auth ssh.AuthMethod
	if i.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(i.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 私钥失败: %w", err)
		}
		auth = ssh.PublicKeys(signer)
	} else if i.Password != "" {
		auth = ssh.Password(i.Password)
	} else {
		return nil, fmt.Errorf("SSH 密码或私钥不能为空")
	}
	hostKey, err := i.hostKeyCallback()
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{User: i.User, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: hostKey, Timeout: i.Timeout}
	if config.Timeout == 0 {
		config.Timeout = 15 * time.Second
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", i.Host, i.Port), config)
}

func (i SSHInstaller) Run(ctx context.Context, command string) (string, error) {
	client, err := i.client(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var output strings.Builder
	session.Stdout = &output
	session.Stderr = &output
	if err := session.Run(command); err != nil {
		return output.String(), fmt.Errorf("远程命令失败: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}

// Install 只接受本地已构建的 agent 二进制与安装脚本，不接受用户任意命令。
func (i SSHInstaller) Install(ctx context.Context, binary []byte, script []byte) (string, error) {
	client, err := i.client(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	if err := uploadBytes(client, "/tmp/havline-agent", binary, 0o755); err != nil {
		return "", err
	}
	if err := uploadBytes(client, "/tmp/havline-agent-install.sh", script, 0o700); err != nil {
		return "", err
	}
	return runSSHSession(client, "sudo /tmp/havline-agent-install.sh /tmp/havline-agent && test -x /usr/local/bin/havline-agent && /usr/local/bin/havline-agent -version && rm -f /tmp/havline-agent /tmp/havline-agent-install.sh")
}

func uploadBytes(client *ssh.Client, path string, data []byte, mode uint32) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	command := fmt.Sprintf("umask 077; cat > %s; chmod %o %s", shellPath(path), mode, shellPath(path))
	if err := session.Start(command); err != nil {
		return err
	}
	if _, err := stdin.Write(data); err != nil {
		return err
	}
	_ = stdin.Close()
	return session.Wait()
}

func runSSHSession(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var output strings.Builder
	session.Stdout = &output
	session.Stderr = &output
	if err := session.Run(command); err != nil {
		return output.String(), fmt.Errorf("远程 agent 安装失败: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}

func shellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

var _ io.Reader = (*os.File)(nil)
var _ = filepath.Separator
