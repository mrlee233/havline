package frp

import (
	"context"
	"fmt"
	"strings"

	agentruntime "github.com/havline/havline/internal/agent"
	"github.com/havline/havline/internal/frp/agentbin"
)

// AgentInstaller 负责把内嵌的 havline-agent 通过 SSH 装到 VPS 上，并收集安装结果。
type AgentInstaller struct {
	SSH SSHInstaller
}

// Probe 返回 VPS 环境摘要：架构、nginx 是否安装、agent 是否已装、systemd 可用性。
func (i AgentInstaller) Probe(ctx context.Context) (map[string]any, error) {
	client, err := i.SSH.client(ctx)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()
	out := func(cmd string) string {
		v, _ := runSSHSession(client, cmd)
		return strings.TrimSpace(v)
	}
	arch := out("uname -m")
	return map[string]any{
		"arch":          arch,
		"nginx_conf":    out(`conf=$(nginx -V 2>&1 | grep -o '\-\-conf-path=[^ ]*' | head -1 | cut -d= -f2-); [ -z "$conf" ] && conf=/etc/nginx/nginx.conf; grep '^[[:space:]]*include' "$conf" 2>/dev/null | grep -m1 -E 'conf\.d|sites-enabled|vhost/nginx' || ls -d /etc/nginx/conf.d 2>/dev/null`),
		"agent_exist":   out("test -x /usr/local/bin/havline-agent && echo yes || echo no") == "yes",
		"agent_running": out("systemctl is-active havline-agent 2>/dev/null || true"),
		"agent_version": out("/usr/local/bin/havline-agent -version 2>/dev/null || echo 未知"),
		// Token 只回「是否已配置 + 末 4 位」：明文不经过 HTTP 返回前端（库里同一字段也是掩码）
		"agent_token_configured": out("test -s /etc/havline-agent/agent.env && grep -q '^HAVLINE_AGENT_TOKEN=' /etc/havline-agent/agent.env && echo yes || echo no") == "yes",
		"agent_token_hint":       out("sed -n 's/^HAVLINE_AGENT_TOKEN=//p' /etc/havline-agent/agent.env 2>/dev/null | tr -d '\\n' | tail -c 4"),
		"frps_alive":             out("systemctl is-active frps 2>/dev/null; pgrep -x frps >/dev/null && echo active || true"),
		"frps_version":           out("/usr/local/bin/frps --version 2>/dev/null || echo 未安装"),
		"frps_config":            out("test -f /etc/frp/frps.toml && echo /etc/frp/frps.toml || echo 配置文件不存在"),
		"frps_unit":              out("systemctl status frps --no-pager 2>/dev/null | head -3 || echo 无 systemd 单位"),
		"frps_log":               out("journalctl -u frps -n 5 --no-pager 2>/dev/null || true"),
		"port7000":               out("ss -tlnp 2>/dev/null | grep ':7000 ' | head -1"),
		"port80":                 out("ss -tlnp 2>/dev/null | grep ':80 ' | head -1"),
		"port443":                out("ss -tlnp 2>/dev/null | grep ':443 ' | head -1"),
	}, nil
}

// Install 上传内嵌二进制并执行安装脚本；重复执行等于覆盖升级。返回 agent Token。
func (i AgentInstaller) Install(ctx context.Context) (string, error) {
	arch, err := i.SSH.Run(ctx, "uname -m")
	if err != nil {
		return "", fmt.Errorf("探测架构失败: %w", err)
	}
	binary, ok := agentbin.BinaryForArch(strings.TrimSpace(arch))
	if !ok {
		return "", fmt.Errorf("不支持的 VPS 架构: %s", strings.TrimSpace(arch))
	}
	// 开发环境（go run / 非 Docker 构建）里内嵌的是占位文件；提前报错而不是让 VPS 上 systemd 反复 Exec format error
	if !isELFBinary(binary) {
		return "", fmt.Errorf("内嵌 agent 二进制无效（开发构建占位文件）。请用 Docker 构建或先执行构建脚本生成真实 agent：go build -o internal/frp/agentbin/havline-agent-linux-amd64 ./cmd/havline-agent（GOOS=linux GOARCH=amd64）")
	}
	script, err := agentInstallScript()
	if err != nil {
		return "", err
	}
	output, err := i.SSH.Install(ctx, binary, script)
	if err != nil {
		return "", err
	}
	token := extractToken(output)
	if token == "" {
		return "", fmt.Errorf("安装完成但未取回 Token: %s", strings.TrimSpace(output))
	}
	version := extractAgentVersion(output)
	if version == "" {
		return "", fmt.Errorf("安装完成但未识别 Agent 版本，无法确认升级结果: %s", strings.TrimSpace(output))
	}
	if version != agentruntime.AgentVersion {
		return "", fmt.Errorf("VPS 上安装的 Agent 版本 %s 与主程序内嵌版本 %s 不一致；请重新构建主程序镜像后再执行「升级 Agent」", version, agentruntime.AgentVersion)
	}
	return token, nil
}

// Uninstall 停止并移除 agent 服务；--keep-data 保留路由与证书。
func (i AgentInstaller) Uninstall(ctx context.Context, keepData bool) (string, error) {
	command := "/usr/local/bin/havline-agent-uninstall"
	if keepData {
		command += " --keep-data"
	}
	out, err := i.SSH.Run(ctx, fmt.Sprintf("if [ -x %s ]; then %s; else echo 'agent 未安装'; fi", "'"+command+"'", command))
	return strings.TrimSpace(out), err
}

// RestorePreviousConfig 在安全迁移验证失败时恢复安装前的 agent.env，并重启旧监听配置。
func (i AgentInstaller) RestorePreviousConfig(ctx context.Context) error {
	command := `if [ -f /etc/havline-agent/agent.env.previous ]; then cp /etc/havline-agent/agent.env.previous /etc/havline-agent/agent.env && chmod 600 /etc/havline-agent/agent.env && systemctl restart havline-agent; fi`
	_, err := i.SSH.Run(ctx, command)
	return err
}

// Diagnose agent 失联时走 SSH 急救：进程、端口、服务日志尾部。
func (i AgentInstaller) Diagnose(ctx context.Context) (string, error) {
	return i.SSH.Run(ctx, "systemctl status havline-agent --no-pager -l 2>/dev/null | head -20; echo '---'; journalctl -u havline-agent -n 15 --no-pager 2>/dev/null; echo '---'; ss -tlnp 2>/dev/null | grep ':7700 ' || echo '7700 not listening'")
}

func extractToken(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if token, found := strings.CutPrefix(strings.TrimSpace(line), "Token: "); found {
			return strings.TrimSpace(token)
		}
	}
	return ""
}

func extractAgentVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if version, found := strings.CutPrefix(strings.TrimSpace(line), "agent version: "); found {
			return strings.TrimSpace(version)
		}
	}
	return ""
}

func agentInstallScript() ([]byte, error) {
	// 与 scripts/agent-install.sh 同一份逻辑内嵌：支持传入二进制路径参数，卸载子命令。
	return []byte(`#!/usr/bin/env bash
set -eu
if [ "$(id -u)" -ne 0 ]; then echo "请使用 root 或 sudo 运行" >&2; exit 1; fi
AGENT_BINARY=${1:-/tmp/havline-agent}
PREFIX=/usr/local/bin/havline-agent
CONF_DIR=/etc/havline-agent
CONF_FILE="$CONF_DIR/agent.env"
UNIT=/etc/systemd/system/havline-agent.service

if [ "${1:-}" = "--uninstall" ]; then
  systemctl stop havline-agent 2>/dev/null || true
  systemctl disable havline-agent 2>/dev/null || true
  rm -f "$UNIT" "$PREFIX" /usr/local/bin/havline-agent-uninstall
  systemctl daemon-reload
  if [ "${2:-}" != "--keep-data" ]; then rm -rf /var/lib/havline-agent; fi
  echo "havline-agent 已卸载"
  exit 0
fi

mkdir -p "$CONF_DIR" /var/lib/havline-agent/certs
# 先停服务，再原子替换；安装后必须确认目标存在、可执行且能输出版本
systemctl stop havline-agent 2>/dev/null || true
TMP_PREFIX="${PREFIX}.new"
# 迁移旧版 Agent 留在 nginx include 范围内的 <配置>.versions 目录（保留历史版本内容）
for NGINX_DIR in /etc/nginx/sites-enabled /etc/nginx/conf.d; do
  [ -d "$NGINX_DIR" ] || continue
  for LEGACY in "$NGINX_DIR"/*.versions; do
    [ -d "$LEGACY" ] || continue
    TARGET="$NGINX_DIR/.$(basename "$LEGACY")"
    [ -e "$TARGET" ] || mv "$LEGACY" "$TARGET" || true
  done
done
install -m 0755 "$AGENT_BINARY" "$TMP_PREFIX"
if [ ! -x "$TMP_PREFIX" ]; then
  echo "agent 二进制安装失败：$TMP_PREFIX 不存在或不可执行" >&2
  exit 1
fi
AGENT_VERSION=$($TMP_PREFIX -version 2>/dev/null) || {
  echo "agent 二进制无法执行：$TMP_PREFIX" >&2
  rm -f "$TMP_PREFIX"
  exit 1
}
mv -f "$TMP_PREFIX" "$PREFIX"
if [ ! -x "$PREFIX" ]; then
  echo "agent 原子替换失败：$PREFIX 不存在或不可执行" >&2
  exit 1
fi
printf 'agent version: %s\n' "$AGENT_VERSION"
if [ ! -f "$CONF_FILE" ]; then
  umask 077
  # 用 /dev/urandom 生成 64 位 hex token，不依赖 openssl（缺失时会静默写入空 token，导致 Havline 无法认证 agent）
  printf 'HAVLINE_AGENT_TOKEN=%s\n' "$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" > "$CONF_FILE"
fi
cp "$CONF_FILE" "$CONF_FILE.previous" 2>/dev/null || true
touch "$CONF_FILE"
sed -i '/^HAVLINE_AGENT_LISTEN=/d; /^HAVLINE_AGENT_ALLOW_REMOTE=/d' "$CONF_FILE"
printf 'HAVLINE_AGENT_LISTEN=127.0.0.1:7700\nHAVLINE_AGENT_ALLOW_REMOTE=0\n' >> "$CONF_FILE"
cat > "$UNIT" <<'UNIT'
[Unit]
Description=Havline FRPS management agent
After=network-online.target nginx.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/havline-agent/agent.env
ExecStart=/usr/local/bin/havline-agent
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl restart havline-agent
systemctl enable havline-agent
cp "$0" /usr/local/bin/havline-agent-uninstall
echo "havline-agent installed"
echo "Token: $(sed -n 's/^HAVLINE_AGENT_TOKEN=//p' "$CONF_FILE")"
`), nil
}

// isELFBinary 判断内容是否为 Linux ELF 可执行（魔数 0x7F 'ELF'），用于拦截开发构建的占位文件。
func isELFBinary(data []byte) bool {
	return len(data) > 4 && data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}
