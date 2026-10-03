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
}
