#!/usr/bin/env bash
# Havline Linux 安装脚本（随 tar.gz 一起分发）
#
#   sudo bash ./install.sh                 安装并启动
#   sudo bash ./install.sh --port 8080     换监听端口
#   sudo bash ./install.sh --uninstall     卸载（保留数据目录）
#   sudo bash ./install.sh --uninstall --purge   卸载并删除数据目录
#
# 用 `bash ./install.sh` 而不是 `./install.sh`：Windows 上打出来的 tar 不带执行位
# （本机 tar 不支持 --mode，待装本时无法保留 +x），显式调 bash 就不需要先 chmod。
#
# 关于初始口令：Havline 首次启动会随机生成一个 16 位管理员口令（用户名 admin），
# 打进攻日志与日志文件（<数据目录>/logs/app.log），本脚本装完会直接把它打印出来。
# 忘了 / 想重新初始化：sudo bash ./install.sh --reset-admin
#
# 安装布局（与 Docker 版同一套环境变量，见 Dockerfile 的 ENV 块）：
#   /opt/havline/havline                 主程序
#   /opt/havline/migrations/*.sql      迁移脚本（程序按工作目录找它，必须与二进制同级）
#   /var/lib/havline/                  数据目录（库、日志、证书、nginx 与 frp 的运行数据）
#   /etc/havline/havline.env              环境变量（含随机生成的会话密钥，0600）
#   /etc/systemd/system/havline.service
set -euo pipefail

PREFIX=/opt/havline
DATA_DIR=/var/lib/havline
ENV_DIR=/etc/havline
ENV_FILE=$ENV_DIR/havline.env
UNIT=/etc/systemd/system/havline.service
SERVICE_NAME=havline
PORT=6893
UNINSTALL=0
PURGE=0
RESET_ADMIN=0

usage() {
  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX=${2:?}; shift 2 ;;
    --data) DATA_DIR=${2:?}; shift 2 ;;
    --port) PORT=${2:?}; shift 2 ;;
    --uninstall) UNINSTALL=1; shift ;;
    --purge) PURGE=1; shift ;;
    --reset-admin) RESET_ADMIN=1; shift ;;
    -h|--help) usage 0 ;;
    *) echo "未知参数：$1" >&2; usage 1 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "请用 root 运行（sudo bash ./install.sh）" >&2
  exit 1
fi

if [ "$UNINSTALL" -eq 1 ]; then
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  systemctl disable "$SERVICE_NAME" 2>/dev/null || true
  rm -f "$UNIT"
  systemctl daemon-reload
  rm -rf "$PREFIX"
  if [ "$PURGE" -eq 1 ]; then
    rm -rf "$DATA_DIR" "$ENV_DIR"
    echo "已卸载，并删除了数据目录 $DATA_DIR 与 $ENV_DIR"
  else
    echo "已卸载。数据目录保留在 $DATA_DIR（如需一并删除：--uninstall --purge）"
  fi
  exit 0
fi

# 从日志里拽出最新一次生成的管理员口令（应用启动时写的是 JSON 行，stdout 与 app.log 都有）
print_initial_password() {
  local line
  line=$(journalctl -u "$SERVICE_NAME" -o cat --no-pager 2>/dev/null \
          | grep -o '"password":"[^"]*"' | tail -n 1)
  if [ -z "$line" ]; then
    line=$(grep -o '"password":"[^"]*"' "$DATA_DIR/logs/app.log" 2>/dev/null | tail -n 1)
  fi
  if [ -n "$line" ]; then
    pw=$(printf '%s' "$line" | sed 's/^"password":"//; s/"$//')
    echo "    初始口令：$pw"
    echo "    用户名：admin（首次登录后请在「设置」里改掉）"
    echo "    这条口令只在首次启动时生成一次，日志文件：$DATA_DIR/logs/app.log"
  else
    echo "    未从日志里读到初始口令（可能不是首次启动）；手动查看："
    echo "      journalctl -u $SERVICE_NAME --no-pager | grep -i password | tail -n 3"
  fi
}

# 重置管理员：删掉 admins 行后重启，启动流程会重新生成一个随机口令并写进日志
reset_admin() {
  if [ ! -f "$DATA_DIR/havline.db" ]; then
    echo "找不到数据库 $DATA_DIR/havline.db，无需重置" >&2
    exit 1
  fi
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  local backup="$DATA_DIR/havline.db.bak.$(date +%Y%m%d-%H%M%S)"
  cp "$DATA_DIR/havline.db" "$backup"
  if command -v sqlite3 >/dev/null 2>&1; then
    sqlite3 "$DATA_DIR/havline.db" 'DELETE FROM sessions; DELETE FROM admins;'
  elif command -v python3 >/dev/null 2>&1; then
    python3 - "$DATA_DIR/havline.db" <<'PY'
import sqlite3, sys
conn = sqlite3.connect(sys.argv[1])
conn.execute('DELETE FROM sessions')
conn.execute('DELETE FROM admins')
conn.commit()
PY
  else
    echo "需要 sqlite3 或 python3 才能改库（已备份到 $backup）" >&2
    systemctl start "$SERVICE_NAME" 2>/dev/null || true
    exit 1
  fi
  echo "==> 已清空管理员与先前的会话（数据库已备份：$backup）"
  systemctl start "$SERVICE_NAME"
  sleep 2
  systemctl is-active --quiet "$SERVICE_NAME" || {
    echo "服务未能启动，请看：journalctl -u $SERVICE_NAME -n 50 --no-pager" >&2
    exit 1
  }
  echo "==> 已重置，下面是新生成的管理员口令："
  print_initial_password
  exit 0
}

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
if [ "$RESET_ADMIN" -eq 1 ]; then
  reset_admin
fi

for f in havline migrations; do
  if [ ! -e "$SCRIPT_DIR/$f" ]; then
    echo "安装包不完整：缺少 $f（请在解压后的目录里运行 ./install.sh）" >&2
    exit 1
  fi
done

# 架构自检：避免把 amd64 包装到 arm64 机器上，起来才报 exec format error
case "$(uname -m)" in
  x86_64) PKG_ARCH=amd64 ;;
  aarch64|arm64) PKG_ARCH=arm64 ;;
  *) PKG_ARCH=unknown ;;
esac
case "$SCRIPT_DIR" in
  *"-linux-amd64") BIN_ARCH=amd64 ;;
  *"-linux-arm64") BIN_ARCH=arm64 ;;
  *) BIN_ARCH=$PKG_ARCH ;;
esac
if [ "$PKG_ARCH" != unknown ] && [ "$BIN_ARCH" != "$PKG_ARCH" ]; then
  echo "警告：当前机器是 $PKG_ARCH，但这个包看起来是 $BIN_ARCH（可能无法运行）" >&2
fi

# nginx 的 mime.types 路径各发行版不同（Debian/Ubuntu 在 /etc/nginx，RHEL 系在 /usr/share/nginx）
MIME_TYPES=/etc/nginx/mime.types
[ -f "$MIME_TYPES" ] || MIME_TYPES=/usr/share/nginx/mime.types

echo "==> 安装到 $PREFIX（数据目录 $DATA_DIR，监听 :$PORT）"
systemctl stop "$SERVICE_NAME" 2>/dev/null || true

install -d -m 0755 "$PREFIX"
install -d -m 0750 "$DATA_DIR"
install -d -m 0750 "$ENV_DIR"
install -d -m 0755 "$PREFIX/migrations"

install -m 0755 "$SCRIPT_DIR/havline" "$PREFIX/havline"
rm -f "$PREFIX"/migrations/*.sql
cp "$SCRIPT_DIR"/migrations/*.sql "$PREFIX/migrations/"
install -m 0644 "$SCRIPT_DIR/VERSION" "$PREFIX/VERSION" 2>/dev/null || true

# 环境变量只在首次安装时生成：重装不会换掉会话密钥（换了所有人都会掉线）
if [ ! -f "$ENV_FILE" ]; then
  SECRET=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
  cat > "$ENV_FILE" <<EOF
# Havline 环境变量（由 install.sh 生成）。修改后：systemctl restart $SERVICE_NAME
HAVLINE_DATA_DIR=$DATA_DIR
HAVLINE_LISTEN=:$PORT
HAVLINE_SESSION_SECRET=$SECRET
HAVLINE_NGINX_MIME_TYPES=$MIME_TYPES
TZ=${TZ:-Asia/Shanghai}
EOF
  chmod 0600 "$ENV_FILE"
  echo "==> 已生成 $ENV_FILE（含随机会话密钥）"
else
  echo "==> 保留已有 $ENV_FILE（如需改端口请编辑它后重启服务）"
fi

cat > "$UNIT" <<EOF
[Unit]
Description=Havline 自托管网关管理面板
Documentation=https://github.com/havline/havline
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
# Havline 会管理同机的 nginx（改配置 / reload / 绑 80·443）并拉起 frpc，需要相应权限
User=root
WorkingDirectory=$PREFIX
EnvironmentFile=$ENV_FILE
ExecStart=$PREFIX/havline
Restart=on-failure
RestartSec=3
# 日志进 journald：journalctl -u $SERVICE_NAME -f
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

sleep 1
if systemctl is-active --quiet "$SERVICE_NAME"; then
  IP=$(hostname -I 2>/dev/null | awk '{print $1}')
  echo
  echo "==> 安装完成，服务已启动"
  echo "    访问：http://${IP:-<本机IP>}:$PORT"
  print_initial_password
  echo "    状态：systemctl status $SERVICE_NAME"
  echo "    日志：journalctl -u $SERVICE_NAME -f"
  echo "    升级：先停止服务，再重跑本脚本（数据与密钥都会保留）"
else
  echo "服务未能启动，请看日志：journalctl -u $SERVICE_NAME -n 50 --no-pager" >&2
  exit 1
fi
