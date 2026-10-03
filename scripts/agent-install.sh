#!/usr/bin/env bash
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 或 sudo 运行" >&2
  exit 1
fi

AGENT_BINARY=${1:-havline-agent}
PREFIX=/usr/local/bin/havline-agent
CONF_DIR=/etc/havline-agent
CONF_FILE="$CONF_DIR/agent.env"
UNIT=/etc/systemd/system/havline-agent.service
mkdir -p "$CONF_DIR" /var/lib/havline-agent/certs
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
  # 用 /dev/urandom 生成 64 位 hex token，不依赖 openssl（与 Havline 内嵌副本保持同一份逻辑）
  printf 'HAVLINE_AGENT_TOKEN=%s\n' "$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')" > "$CONF_FILE"
fi

# 默认只监听回环；显式删除旧公网监听，由中央 Havline 通过 SSH 隧道访问。
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
User=root

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl restart havline-agent
systemctl enable havline-agent
TOKEN=$(sed -n 's/^HAVLINE_AGENT_TOKEN=//p' "$CONF_FILE")
echo "havline-agent 已安装并启动"
echo "API: SSH tunnel -> 127.0.0.1:7700"
echo "Token: $TOKEN"
