package api

import (
	"io/fs"
	"log/slog"

	"github.com/fonu/fonu/internal/acme"
	"github.com/fonu/fonu/internal/auth"
	"github.com/fonu/fonu/internal/backup"
	"github.com/fonu/fonu/internal/chinacidr"
	"github.com/fonu/fonu/internal/cloudflared"
	"github.com/fonu/fonu/internal/config"
	"github.com/fonu/fonu/internal/ddns"
	"github.com/fonu/fonu/internal/discovery"
	"github.com/fonu/fonu/internal/frp"
	"github.com/fonu/fonu/internal/metrics"
	"github.com/fonu/fonu/internal/notify"
	"github.com/fonu/fonu/internal/service"
	"github.com/fonu/fonu/internal/settings"
	"github.com/fonu/fonu/internal/traffic"
	"github.com/fonu/fonu/internal/updater"
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
