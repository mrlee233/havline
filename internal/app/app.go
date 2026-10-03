package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/havline/havline/internal/acme"
	"github.com/havline/havline/internal/api"
	"github.com/havline/havline/internal/auth"
	"github.com/havline/havline/internal/backup"
	"github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/chinacidr"
	"github.com/havline/havline/internal/cloudflared"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
	"github.com/havline/havline/internal/ddns"
	"github.com/havline/havline/internal/discovery"
	"github.com/havline/havline/internal/frp"
	"github.com/havline/havline/internal/logstore"
	"github.com/havline/havline/internal/metrics"
	"github.com/havline/havline/internal/monitor"
	"github.com/havline/havline/internal/nginx"
	"github.com/havline/havline/internal/notify"
	"github.com/havline/havline/internal/proxy"
	"github.com/havline/havline/internal/scheduler"
	"github.com/havline/havline/internal/secret"
	"github.com/havline/havline/internal/service"
	"github.com/havline/havline/internal/settings"
	"github.com/havline/havline/internal/traffic"
	"github.com/havline/havline/internal/updater"
)

type App struct {
	cfg        config.Config
	logger     *slog.Logger
	server     *http.Server
	nginx      *nginx.Manager
	frpMulti   *frp.Service
	cloudflare *cloudflared.Service
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

	// 会话密钥：显式配置优先；未配置则在数据目录里读/生成并持久化
	// （它同时是 secret box 的加密密钥，所以只在未配置时才生成，不替换已配置的值）
	secretSource, err := cfg.ResolveSessionSecret()
	if err != nil {
		return nil, err
	}
	logger.Info("会话密钥已就绪", "module", "SYSTEM", "source", secretSource)

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
	// 指标历史：与巡检同生命周期，采样跟随巡检轮次
	metricsStore := metrics.NewStore(conn)
	discoverySvc := discovery.New()
	// 新增：多服务端 FRP 模块（接管原网关模式）
	frpMultiSvc := frp.NewService(cfg, frp.NewStore(conn), secretBox, logger, certStore)
	if err := frpMultiSvc.EnsureDirs(); err != nil {
		conn.Close()
		return nil, err
	}
	cloudflareSvc := cloudflared.NewService(cfg, cloudflared.NewStore(conn), secretBox, logger, proxySvc)
	if err := cloudflareSvc.EnsureDirs(); err != nil {
		conn.Close()
		return nil, err
	}
	proxySvc.SetExitSync(cloudflareSvc)
	startedAt := time.Now().UTC().Format(time.RFC3339)
	trafficCollector := traffic.NewCollector(conn, filepath.Join(cfg.LogsDir(), "access.log"), notifySvc)
	updaterClient := updater.NewClient(cfg.UpdaterSocket, cfg.UpdaterToken)

	handler := api.NewRouter(api.Deps{
		Config:     cfg,
		Logger:     logger,
		Auth:       authSvc,
		Proxy:      proxySvc,
		DDNS:       ddnsSvc,
		ACME:       acmeSvc,
		Settings:   settingsStore,
		Notify:     notifySvc,
		ChinaCIDR:  chinaCIDRSvc,
		Backup:     backupSvc,
		Metrics:    metricsStore,
		Discovery:  discoverySvc,
		FRPMulti:   frpMultiSvc,
		Cloudflare: cloudflareSvc,
		Traffic:    trafficCollector,
		Updater:    updaterClient,
		StaticFS:   staticFS,
		StartedAt:  startedAt,
	})
	if prefix := strings.TrimSpace(cfg.GatewayPrefix); prefix != "" {
		handler = stripPrefixIfPresent(prefix, handler)
	}
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
	go frpMultiSvc.StartAgentTunnels(ctx)
	go frpMultiSvc.StartAgentCertificateRenewal(ctx)
	go cloudflareSvc.StartAll(ctx)
	intervalMinutes, _ := settingsStore.GetInt(ctx, settings.KeyDDNSCheckInterval)
	if intervalMinutes <= 0 {
		intervalMinutes = 5
	}
	chinaHours, _ := settingsStore.GetInt(ctx, settings.KeyChinaCIDRUpdateHours)
	if chinaHours <= 0 {
		chinaHours = 24
	}
	// 巡检间隔比其它任务短：「连续 N 次不通」需要足够采样次数才能判定
	monitorChecker := monitor.New(settingsStore, notifySvc, logger, monitorSource{frp: frpMultiSvc, cloudflare: cloudflareSvc, logger: logger, traffic: trafficCollector}, healthRecorder{frp: frpMultiSvc}, metricsStore)
	// 自动备份：启动时会先跑一轮，靠「最新备份是否在 24h 内」避免频繁重启重复备份
	backupRunner := backup.NewAutoRunner(backupSvc, settingsStore, notifySvc, logger)
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
		scheduler.Job{
			Name:     "backup",
			Interval: backup.AutoInterval,
			Run:      backupRunner.Run,
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
	if strings.TrimSpace(a.cfg.GatewaySocket) == "" {
		return a.server.ListenAndServe()
	}
	// 先准备两端监听，任何一端失败都不留下半启动的服务。
	tcp, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return err
	}
	defer tcp.Close()
	socket := strings.TrimSpace(a.cfg.GatewaySocket)
	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o660); err != nil {
		return err
	}
	errCh := make(chan error, 2)
	go func() { errCh <- a.server.Serve(tcp) }()
	go func() { errCh <- a.server.Serve(listener) }()
	return <-errCh
}

func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("application shutting down", "module", "SYSTEM")
	if a.shutdownFn != nil {
		a.shutdownFn()
	}
	if err := a.server.Shutdown(ctx); err != nil {
		return err
	}
	if socket := strings.TrimSpace(a.cfg.GatewaySocket); socket != "" {
		_ = os.Remove(socket)
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

func stripPrefixIfPresent(prefix string, next http.Handler) http.Handler {
	prefix = strings.TrimRight(prefix, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}
		next.ServeHTTP(w, r)
	})
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
