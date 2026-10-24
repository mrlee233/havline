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
	"github.com/havline/havline/internal/terminal"
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

	// updater Token 只给「一键升级」sidecar 用；原生版与飞牛 Docker 版都显式把 socket 置空，
	// 此时不能去写 /run/havline-updater：原生版以非 root 运行，权限不足会让应用直接起不来。
	updaterToken, updaterTokenSource, err := updater.EnsureTokenIfEnabled(cfg.UpdaterSocket, cfg.UpdaterToken, cfg.UpdaterTokenFile)
	if err != nil {
		return nil, err
	}
	if updaterToken != "" {
		logger.Info("updater Token 已就绪", "module", "SYSTEM", "source", updaterTokenSource)
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
	// 时区必须在任何业务开始前生效：通知时间戳、日志时间、调度判断都依赖 time.Local。
	// 设置项缺失或写坏时退回 TZ 环境变量，不让应用起不来。
	if loc, err := settings.ApplyTimezone(context.Background(), settingsStore); err != nil {
		logger.Warn("应用时区设置失败，沿用系统 TZ", "module", "SYSTEM", "error", err.Error())
	} else {
		logger.Info("时区已应用", "module", "SYSTEM", "timezone", loc.String())
	}
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
	// 注入数据库连接：导出时用 VACUUM INTO 拿一致性快照，而不是直接打包正在写的文件
	backupSvc.SetDB(conn)
	// 指标历史：与巡检同生命周期，采样跟随巡检轮次
	metricsStore := metrics.NewStore(conn)
	discoverySvc := discovery.New()
	// 新增：多服务端 FRP 模块（接管原网关模式）
	frpMultiSvc := frp.NewService(cfg, frp.NewStore(conn), secretBox, logger, certStore)
	if err := frpMultiSvc.EnsureDirs(); err != nil {
		conn.Close()
		return nil, err
	}
	// 记录/恢复 frpc 运行状态：应用被停用再启用后要能自动回到运行状态
	frpMultiSvc.SetSettingsStore(settingsStore)
	cloudflareSvc := cloudflared.NewService(cfg, cloudflared.NewStore(conn), secretBox, logger, proxySvc)
	if err := cloudflareSvc.EnsureDirs(); err != nil {
		conn.Close()
		return nil, err
	}
	proxySvc.SetExitSync(cloudflareSvc)
	startedAt := time.Now().UTC().Format(time.RFC3339)
	trafficCollector := traffic.NewCollector(conn, filepath.Join(cfg.LogsDir(), "access.log"), notifySvc)
	// 删规则时连带清掉它的流量累计，否则同名规则重建会继承上一轮的读数
	proxySvc.SetTrafficCleaner(trafficCollector)
	updaterClient := updater.NewClient(cfg.UpdaterSocket, updaterToken)
	// 内置 SSH 终端：会话管理借用 frp 的 SSH 凭据与 known_hosts；开关默认关闭，见 settings.KeyTerminalEnabled
	// 会话审计落 SQLite：谁、何时、连了哪台、多久、为什么断开
	terminalAuditStore := terminal.NewAuditStore(conn)
	// 常用命令：右侧面板的快捷命令，落库后换浏览器也在
	terminalCommandStore := terminal.NewCommandStore(conn)
	terminalManager := terminal.NewManager(logger, frpMultiSvc.OpenSSHTerminal, terminal.Options{})
	terminalManager.SetAuditSink(terminalAuditStore)
	// 会话录制：默认关闭，开启后按 asciinema v2 把输出写进 logs/terminal
	terminalRecordingStore := terminal.NewRecordingStore(filepath.Join(cfg.LogsDir(), "terminal"))
	if err := terminalRecordingStore.EnsureDir(); err != nil {
		logger.Warn("终端录制目录创建失败", "module", "TERMINAL", "error", err.Error())
	}
	terminalManager.SetRecorderFactory(func(sessionID string, opts terminal.CreateOptions) (terminal.OutputRecorder, error) {
		title := fmt.Sprintf("服务端 #%d", opts.ServerID)
		return terminal.NewCastRecorder(terminalRecordingStore, sessionID, title, opts.Cols, opts.Rows, time.Now())
	})

	handler := api.NewRouter(api.Deps{
		Config:             cfg,
		Logger:             logger,
		Auth:               authSvc,
		Proxy:              proxySvc,
		DDNS:               ddnsSvc,
		ACME:               acmeSvc,
		Settings:           settingsStore,
		Notify:             notifySvc,
		ChinaCIDR:          chinaCIDRSvc,
		Backup:             backupSvc,
		Metrics:            metricsStore,
		Discovery:          discoverySvc,
		FRPMulti:           frpMultiSvc,
		Cloudflare:         cloudflareSvc,
		Traffic:            trafficCollector,
		Updater:            updaterClient,
		StaticFS:           staticFS,
		StartedAt:          startedAt,
		Terminal:           terminalManager,
		TerminalAudits:     terminalAuditStore,
		TerminalCommands:   terminalCommandStore,
		TerminalRecordings: terminalRecordingStore,
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
	// 终端会话回收：空闲超时、超长会话与已结束会话都靠它清理
	terminalManager.StartSweeper(ctx, time.Minute)
	frpMultiSvc.SetBaseContext(ctx)
	// cloudflared 子进程跟随应用 ctx：应用退出时不会留下孤儿进程
	cloudflareSvc.SetBaseContext(ctx)
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
		scheduler.Job{
			Name:     "terminal_audit",
			Interval: 24 * time.Hour,
			// 审计只留 90 天：清理比会话回收慢得多，单独挂一个日任务即可
			Run: func(ctx context.Context) {
				deleted, err := terminalAuditStore.PruneAudits(ctx)
				if err != nil {
					logger.Warn("终端审计清理失败", "module", "TERMINAL", "error", err.Error())
				}
				if deleted > 0 {
					logger.Info("终端审计清理完成", "module", "TERMINAL", "deleted", deleted)
				}
				removed, err := terminalRecordingStore.Prune(time.Now().Add(-terminal.AuditRetention))
				if err != nil {
					logger.Warn("终端录制清理失败", "module", "TERMINAL", "error", err.Error())
				}
				if removed > 0 {
					logger.Info("终端录制清理完成", "module", "TERMINAL", "deleted", removed)
				}
			},
		},
		scheduler.Job{
			Name:     "session_prune",
			Interval: 24 * time.Hour,
			// 过期会话否则只能等自己被访问时删掉自己，长期不登录的设备会一直留在表里
			Run: func(ctx context.Context) {
				removed, err := authSvc.PruneExpiredSessions(ctx)
				if err != nil {
					logger.Warn("过期会话清理失败", "module", "AUTH", "error", err.Error())
					return
				}
				if removed > 0 {
					logger.Info("过期会话清理完成", "module", "AUTH", "deleted", removed)
				}
			},
		},
		scheduler.Job{
			Name:     "log_rotate",
			Interval: time.Hour,
			// Nginx 自己不会轮转：不切的话 access.log 会一直长，同时读日志的
			// 几个接口（今日统计、小时曲线）会越来越慢。
			Run: func(ctx context.Context) {
				rotateNginxLogs(ctx, cfg, nginxMgr, settingsStore, logger)
			},
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
		cloudflare: cloudflareSvc,
		scheduler:  sched,
		shutdownFn: cancel,
		db:         conn,
	}

	// 就绪门：Nginx 首轮配置应用完之前不放后台任务。
	// 否则 Agent 路由下发、Cloudflare ingress 同步会在启动瞬间各触发一次
	// Nginx reload，和 bootstrap 抢同一份配置与 pid。
	ready := make(chan struct{})
	if err := app.bootstrapNginx(context.Background(), proxySvc); err != nil {
		cancel()
		conn.Close()
		return nil, err
	}
	close(ready)
	go waitReady(ctx, ready, func() {
		if err := frpMultiSvc.ReconcileRuntime(ctx); err != nil {
			logger.Warn("initial frp runtime reconcile skipped", "module", "FRP", "error", err.Error())
		}
		if err := frpMultiSvc.AutoStart(ctx); err != nil {
			logger.Warn("initial frp autostart skipped", "module", "FRP", "error", err.Error())
		}
		cloudflareSvc.StartAuto(ctx)
	})
	go waitReady(ctx, ready, func() { frpMultiSvc.StartAgentTunnels(ctx) })
	go waitReady(ctx, ready, func() { frpMultiSvc.StartAgentCertificateRenewal(ctx) })

	listen := strings.TrimSpace(cfg.ListenAddr)
	if listen == "" {
		listen = strings.TrimSpace(cfg.GatewaySocket)
	}
	logger.Info("application started", "module", "SYSTEM", "listen", listen)
	return app, nil
}

// waitReady 等到 ready 关闭（或 ctx 取消）后再执行 fn。
func waitReady(ctx context.Context, ready <-chan struct{}, fn func()) {
	select {
	case <-ready:
	case <-ctx.Done():
		return
	}
	if ctx.Err() != nil {
		return
	}
	fn()
}

// rotateNginxLogs 轮转 access.log 与 error.log。
// 保留天数接「设置 → 常规 → 日志保留天数」，份数用 logstore 默认值兜底。
func rotateNginxLogs(ctx context.Context, cfg config.Config, nginxMgr *nginx.Manager, store *settings.Store, logger *slog.Logger) {
	retentionDays, err := store.GetInt(ctx, settings.KeyLogRetentionDays)
	if err != nil || retentionDays <= 0 {
		retentionDays = 30
	}
	logsDir := cfg.LogsDir()
	for _, name := range []string{"access.log", "error.log"} {
		rotator := logstore.NewRotator(filepath.Join(logsDir, name), retentionDays, func() error {
			return nginxMgr.ReopenLogs(ctx)
		})
		rotated, err := rotator.RotateIfNeeded(time.Now())
		if err != nil {
			logger.Warn("日志轮转失败", "module", "LOGSTORE", "file", name, "error", err.Error())
			continue
		}
		if rotated {
			logger.Info("日志已轮转", "module", "LOGSTORE", "file", name)
		}
	}
}

func (a *App) bootstrapNginx(ctx context.Context, proxySvc *service.ProxyService) error {
	if err := proxySvc.ReloadAll(ctx); err != nil {
		a.logger.Warn("initial nginx apply skipped", "module", "NGINX", "error", err.Error())
	}
	return nil
}

func (a *App) Run() error {
	socket := strings.TrimSpace(a.cfg.GatewaySocket)
	if socket == "" {
		return a.server.ListenAndServe()
	}

	listeners := make([]net.Listener, 0, 2)
	// 原生版显式把 TCP 地址置空，只保留 Unix Socket，避免和 Docker 版抢 6893。
	if addr := strings.TrimSpace(a.server.Addr); addr != "" {
		tcp, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		defer tcp.Close()
		listeners = append(listeners, tcp)
	}

	if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return err
	}
	unixListener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer unixListener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o660); err != nil {
		return err
	}
	listeners = append(listeners, unixListener)

	errCh := make(chan error, len(listeners))
	for _, listener := range listeners {
		go func(l net.Listener) { errCh <- a.server.Serve(l) }(listener)
	}
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
		// 退出时保留「上次在运行」标记，供下次启用应用时自动恢复 frpc
		if err := a.frpMulti.StopForShutdown(ctx); err != nil {
			a.logger.Error("frpc stop failed", "module", "FRP", "error", err.Error())
		}
	}
	// cloudflared 不保留「上次在运行」：AutoStart 是用户的显式选择，
	// 这里只负责把当前子进程收干净，避免退出后残留进程占着 Cloudflare 连接。
	// 放在 nginx 之后：ingress 还指向本机 Nginx，先收隧道再做别的没有意义。
	if a.cloudflare != nil {
		a.cloudflare.StopAll(ctx)
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
