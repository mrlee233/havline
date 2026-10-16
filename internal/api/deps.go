package api

import (
	"io/fs"
	"log/slog"

	"github.com/havline/havline/internal/acme"
	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/backup"
	"github.com/havline/havline/internal/chinacidr"
	"github.com/havline/havline/internal/cloudflared"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/ddns"
	"github.com/havline/havline/internal/discovery"
	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/metrics"
	"github.com/havline/havline/internal/notify"
	"github.com/havline/havline/internal/service"
	"github.com/havline/havline/internal/settings"
	"github.com/havline/havline/internal/terminal"
	"github.com/havline/havline/internal/traffic"
	"github.com/havline/havline/internal/updater"
)

type Deps struct {
	Config     config.Config
	Logger     *slog.Logger
	Auth       *auth.Service
	Proxy      *service.ProxyService
	DDNS       *ddns.Service
	ACME       *acme.Service
	Settings   *settings.Store
	ChinaCIDR  *chinacidr.Service
	Backup     *backup.Service
	Metrics    *metrics.Store
	Discovery  *discovery.Service
	FRPMulti   *frp.Service
	Cloudflare *cloudflared.Service
	Traffic    *traffic.Collector
	Notify     *notify.Service
	Updater    *updater.Client
	StaticFS   fs.FS
	StartedAt  string
	// Terminal 是内置 SSH 终端的会话管理器；为 nil 时相关接口不注册
	Terminal *terminal.Manager
	// TerminalAudits 是终端会话审计存储；为 nil 时不注册审计接口
	TerminalAudits *terminal.AuditStore
	// TerminalCommands 是终端常用命令存储；为 nil 时常用命令接口返回空
	TerminalCommands *terminal.CommandStore
	// TerminalRecordings 是会话录制存储；为 nil 时录制接口返回空
	TerminalRecordings *terminal.RecordingStore
}
