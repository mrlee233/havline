package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fonu/fonu/internal/acme"
	"github.com/fonu/fonu/internal/api"
	"github.com/fonu/fonu/internal/auth"
	"github.com/fonu/fonu/internal/backup"
	"github.com/fonu/fonu/internal/certificate"
	"github.com/fonu/fonu/internal/chinacidr"
	"github.com/fonu/fonu/internal/config"
	"github.com/fonu/fonu/internal/db"
	"github.com/fonu/fonu/internal/ddns"
	"github.com/fonu/fonu/internal/discovery"
	"github.com/fonu/fonu/internal/frp"
	"github.com/fonu/fonu/internal/logstore"
	"github.com/fonu/fonu/internal/monitor"
	"github.com/fonu/fonu/internal/nginx"
	"github.com/fonu/fonu/internal/notify"
	"github.com/fonu/fonu/internal/proxy"
	"github.com/fonu/fonu/internal/scheduler"
	"github.com/fonu/fonu/internal/secret"
	"github.com/fonu/fonu/internal/service"
	"github.com/fonu/fonu/internal/settings"
	"github.com/fonu/fonu/internal/traffic"
)

type App struct {
	cfg        config.Config
	logger     *slog.Logger
	server     *http.Server
	nginx      *nginx.Manager
	frpMulti   *frp.Service
	scheduler  *scheduler.Scheduler
	shutdownFn context.CancelFunc
	db         interface{ Close() error }
}

func New(cfg config.Config, staticFS fs.FS, migrationsDir string) (*App, error) {
	appLogWriter := logstore.NewAppLogWriter(cfg.LogsDir())
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, appLogWriter), &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	for _, dir := range []string{cfg.DataDir, cfg.NginxDir(), cfg.LogsDir(), cfg.CertsDir(), cfg.FrpDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	conn, err := db.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(context.Background(), conn, migrationsDir); err != nil {
		conn.Close()
		return nil, err
	}

	secretBox, err := secret.NewBox(cfg.SessionSecret)
	if err != nil {
		conn.Close()
		return nil, err
	}

	authSvc := auth.New(conn)
	if err := authSvc.EnsureDefaultAdmin(context.Background(), logger); err != nil {
		conn.Close()
		return nil, fmt.Errorf("bootstrap admin: %w", err)
	}
	proxyStore := proxy.NewStore(conn)
	settingsStore := settings.NewStore(conn)
	ddnsStore := ddns.NewStore(conn)
	certStore := certificate.NewStore(conn)
	nginxMgr := nginx.NewManager(cfg, logger.With("module", "NGINX"))
	if err := nginxMgr.EnsureDirs(); err != nil {
		conn.Close()
		return nil, err
	}

	proxySvc := service.NewProxyService(cfg, conn, proxyStore, certStore, settingsStore, nginxMgr)
	chinaCIDRSvc := chinacidr.New(cfg, settingsStore, nginxMgr, proxyStore, logger.With("module", "CHINA_CIDR"))
	notifySvc := notify.New(settingsStore, secretBox, logger)
	proxySvc.SetNotify(notifySvc)
	ddnsSvc := ddns.NewService(ddnsStore, settingsStore, secretBox, logger, notifySvc)
	acmeSvc := acme.NewService(cfg, certStore, ddnsSvc, settingsStore, proxySvc, logger, notifySvc)
	backupSvc := backup.New(cfg.DataDir)
	discoverySvc := discovery.New()
	// 新增：多服务端 FRP 模块（接管原网关模式）
	frpMultiSvc := frp.NewService(cfg, frp.NewStore(conn), secretBox, logger, certStore)
	if err := frpMultiSvc.EnsureDirs(); err != nil {
		conn.Close()
		return nil, err
	}
	startedAt := time.Now().UTC().Format(time.RFC3339)
	trafficCollector := traffic.NewCollector(conn, filepath.Join(cfg.LogsDir(), "access.log"), notifySvc)

	handler := api.NewRouter(api.Deps{
		Config:    cfg,
		Logger:    logger,
		Auth:      authSvc,
		Proxy:     proxySvc,
		DDNS:      ddnsSvc,
		ACME:      acmeSvc,
		Settings:  settingsStore,
		Notify:    notifySvc,
		ChinaCIDR: chinaCIDRSvc,
		Backup:    backupSvc,
		Discovery: discoverySvc,
		FRPMulti:  frpMultiSvc,
		Traffic:   trafficCollector,
		StaticFS:  staticFS,
		StartedAt: startedAt,
	})
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// 读超时放到 5 分钟：备份恢复要上传最大 64MB；不设 WriteTimeout，否则日志 SSE 会被中途切断
		ReadTimeout: 5 * time.Minute,
		IdleTimeout: 2 * time.Minute,
	}

	ctx, cancel := context.WithCancel(context.Background())
	frpMultiSvc.SetBaseContext(ctx)
	intervalMinutes, _ := settingsStore.GetInt(ctx, settings.KeyDDNSCheckInterval)
	if intervalMinutes <= 0 {
		intervalMinutes = 5
	}
	chinaHours, _ := settingsStore.GetInt(ctx, settings.KeyChinaCIDRUpdateHours)
	if chinaHours <= 0 {
		chinaHours = 24
	}
	// 资源与隧道巡检：间隔比其它任务短，「连续 N 次不通」需要足够采样次数才能判定
	monitorChecker := monitor.New(settingsStore, notifySvc, logger, func(ctx context.Context) ([]monitor.RouteStatus, error) {
		servers, err := frpMultiSvc.ListServers(ctx)
		if err != nil {
			return nil, err
		}
		routes := make([]monitor.RouteStatus, 0, len(servers))
		for _, server := range servers {
			// 单台服务端探测失败不影响其它服务端：它本轮跳过，等下一轮
			health, err := frpMultiSvc.RouteHealth(ctx, server.ID)
			if err != nil {
				logger.Warn("巡检跳过服务端", "module", "MONITOR", "server_id", server.ID, "error", err.Error())
				continue
			}
			for _, item := range health {
				routes = append(routes, monitor.RouteStatus{Domain: item.Domain, TunnelOK: item.TunnelOK, Detail: item.TunnelDetail})
			}
		}
		return routes, nil
	})
	sched := scheduler.New(
		scheduler.Job{
			Name:     "ddns",
			Interval: time.Duration(intervalMinutes) * time.Minute,
			Run:      ddnsSvc.Tick,
		},
		scheduler.Job{
			Name:     "acme",
			Interval: 24 * time.Hour,
			Run:      acmeSvc.Tick,
		},
		scheduler.Job{
			Name:     "china_cidr",
			Interval: time.Duration(chinaHours) * time.Hour,
			Run:      chinaCIDRSvc.Tick,
		},
		scheduler.Job{
			Name:     "monitor",
			Interval: 2 * time.Minute,
			Run:      monitorChecker.Run,
		},
	)
	sched.Start(ctx)
	trafficCollector.Start(ctx)

	app := &App{
		cfg:        cfg,
		logger:     logger,
		server:     server,
		nginx:      nginxMgr,
		frpMulti:   frpMultiSvc,
		scheduler:  sched,
		shutdownFn: cancel,
		db:         conn,
	}

	if err := app.bootstrapNginx(context.Background(), proxySvc); err != nil {
		cancel()
		conn.Close()
		return nil, err
	}
	if err := frpMultiSvc.ReconcileRuntime(context.Background()); err != nil {
		logger.Warn("initial frp runtime reconcile skipped", "module", "FRP", "error", err.Error())
	}
	if err := frpMultiSvc.AutoStart(context.Background()); err != nil {
		logger.Warn("initial frp autostart skipped", "module", "FRP", "error", err.Error())
	}

	logger.Info("application started", "module", "SYSTEM", "listen", cfg.ListenAddr)
	return app, nil
}

func (a *App) bootstrapNginx(ctx context.Context, proxySvc *service.ProxyService) error {
	if err := proxySvc.ReloadAll(ctx); err != nil {
		a.logger.Warn("initial nginx apply skipped", "module", "NGINX", "error", err.Error())
	}
	return nil
}

func (a *App) Run() error {
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("application shutting down", "module", "SYSTEM")
	if a.shutdownFn != nil {
		a.shutdownFn()
	}
	if err := a.server.Shutdown(ctx); err != nil {
		return err
	}
	if err := a.nginx.Stop(ctx); err != nil {
		a.logger.Error("nginx stop failed", "module", "NGINX", "error", err.Error())
	}
	if a.frpMulti != nil {
		if err := a.frpMulti.Stop(ctx); err != nil {
			a.logger.Error("frpc stop failed", "module", "FRP", "error", err.Error())
		}
	}
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func ResolveMigrationsDir() (string, error) {
	candidates := []string{
		"migrations",
		filepath.Join("..", "migrations"),
		filepath.Join("..", "..", "migrations"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("migrations directory not found")
}
