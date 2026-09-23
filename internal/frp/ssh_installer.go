package frp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHInstaller 通过固定白名单命令在 VPS 上安装/升级 fonu-agent。
// agent 的日常管理仍走 AgentClient；SSH 只承担生命周期和失联自救。
type SSHInstaller struct {
	Host       string
	Port       int
	User       string
	Password   string
	PrivateKey string
	Timeout    time.Duration
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
	config := &ssh.ClientConfig{User: i.User, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: i.Timeout}
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
	if err := uploadBytes(client, "/tmp/fonu-agent", binary, 0o755); err != nil {
		return "", err
	}
	if err := uploadBytes(client, "/tmp/fonu-agent-install.sh", script, 0o700); err != nil {
		return "", err
	}
	return runSSHSession(client, "sudo /tmp/fonu-agent-install.sh /tmp/fonu-agent && test -x /usr/local/bin/fonu-agent && /usr/local/bin/fonu-agent -version && rm -f /tmp/fonu-agent /tmp/fonu-agent-install.sh")
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
