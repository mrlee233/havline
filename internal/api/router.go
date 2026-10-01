package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/havline/havline/internal/auth"
)

// routeSpec 是一条 API 路由的注册信息；测试据此遍历全部路由验证鉴权覆盖。
type routeSpec struct {
	Pattern string
	Handler http.HandlerFunc
	Public  bool
}

func NewRouter(deps Deps) http.Handler {
	handler, _ := newRouterWithSpecs(deps)
	return handler
}

func newRouterWithSpecs(deps Deps) (http.Handler, []routeSpec) {
	logsHandler := NewLogsHandler(deps.Config)
	r := &Router{
		authHandler:       NewAuthHandler(deps.Auth, deps.Notify),
		proxyHandler:      NewProxyHandler(deps.Config, deps.Proxy, logsHandler, deps.Traffic),
		nginxHandler:      NewNginxHandler(deps.Proxy),
		statusHandler:     NewStatusHandler(deps.Config, deps.Proxy, deps.DDNS, deps.ACME, deps.Cloudflare, deps.StartedAt, deps.Config.NginxPIDFile),
		ddnsHandler:       NewDDNSHandler(deps.DDNS),
		certHandler:       NewCertHandler(deps.ACME),
		settingsHandler:   NewSettingsHandler(deps.Settings, deps.Proxy, deps.Notify),
		chinaCIDRHandler:  NewChinaCIDRHandler(deps.ChinaCIDR),
		logsHandler:       logsHandler,
		backupHandler:     NewBackupHandler(deps.Backup, deps.Settings),
		statusPageHandler: NewStatusPageHandler(deps.Settings, deps.FRPMulti, deps.Cloudflare),
		metricsHandler:    NewMetricsHandler(deps.Metrics),
		discoveryHandler:  NewDiscoveryHandler(deps.Discovery),
		frpmultiHandler:   NewFRPMultiHandler(deps.FRPMulti, deps.Auth),
		cloudflareHandler: NewCloudflareHandler(deps.Cloudflare),
		updateHandler:     NewUpdateHandler(deps.Updater),
		authSvc:           deps.Auth,
		staticFS:          deps.StaticFS,
	}

	mux := http.NewServeMux()
	var specs []routeSpec
	public := func(pattern string, handler http.HandlerFunc) {
		specs = append(specs, routeSpec{Pattern: pattern, Handler: handler, Public: true})
	}
	protected := func(pattern string, handler http.HandlerFunc) {
		specs = append(specs, routeSpec{Pattern: pattern, Handler: handler})
	}

	public("GET /api/version", Version(deps.Config))
	// 唯一一个匿名只读接口：默认关闭、可设访问码、按 IP 限流
	public("GET /api/status-page", r.statusPageHandler.Get)
	public("GET /api/auth/status", r.authHandler.Status)
	public("POST /api/auth/login", r.authHandler.Login)
	public("POST /api/auth/logout", r.authHandler.Logout)

	protect := protected

	protect("POST /api/auth/password", r.authHandler.ChangePassword)
	protect("GET /api/settings/tokens", r.authHandler.ListTokens)
	protect("POST /api/settings/tokens", r.authHandler.CreateToken)
	protect("DELETE /api/settings/tokens/{id}", r.authHandler.DeleteToken)
	protect("GET /api/status", r.statusHandler.Get)
	protect("GET /api/proxies", r.proxyHandler.List)
	protect("GET /api/proxies/traffic", r.proxyHandler.Traffic)
	protect("POST /api/proxies", r.proxyHandler.Create)
	protect("PUT /api/proxies/reorder", r.proxyHandler.Reorder)
	protect("PUT /api/proxies/{id}", r.proxyHandler.Update)
	protect("DELETE /api/proxies/{id}", r.proxyHandler.Delete)
	protect("GET /api/proxies/{id}/logs/stream", r.proxyHandler.StreamLogs)
	protect("GET /api/proxies/{id}/clients", r.proxyHandler.Clients)
	protect("GET /api/proxies/{id}/nginx", r.nginxHandler.GetRule)
	protect("PUT /api/proxies/{id}/nginx", r.nginxHandler.PutRule)
	protect("POST /api/proxies/{id}/nginx/rollback", r.nginxHandler.RollbackRule)
	protect("GET /api/settings/nginx/global", r.nginxHandler.GetGlobal)
	protect("PUT /api/settings/nginx/global", r.nginxHandler.PutGlobal)
	protect("POST /api/settings/nginx/global/rollback", r.nginxHandler.RollbackGlobal)
	protect("GET /api/settings/nginx/config/versions", r.nginxHandler.GetConfigVersions)
	protect("GET /api/settings/nginx/config/versions/{name}", r.nginxHandler.GetConfigVersion)
	protect("POST /api/settings/nginx/config/versions/{name}/rollback", r.nginxHandler.RollbackConfigVersion)
	protect("GET /api/ddns", r.ddnsHandler.List)
	protect("POST /api/ddns", r.ddnsHandler.Create)
	protect("PUT /api/ddns/{id}", r.ddnsHandler.Update)
	protect("DELETE /api/ddns/{id}", r.ddnsHandler.Delete)
	protect("POST /api/ddns/test", r.ddnsHandler.Test)
	protect("POST /api/ddns/update", r.ddnsHandler.UpdateAll)
	protect("POST /api/ddns/{id}/update", r.ddnsHandler.UpdateOne)
	protect("GET /api/certificates", r.certHandler.List)
	protect("GET /api/certificates/ca-options", r.certHandler.Options)
	protect("POST /api/certificates/apply", r.certHandler.Apply)
	protect("GET /api/certificates/jobs/{id}/stream", r.certHandler.ApplyJobStream)
	protect("POST /api/certificates/import", r.certHandler.Import)
	protect("POST /api/certificates/renew", r.certHandler.Renew)
	protect("GET /api/certificates/{domain}/download", r.certHandler.Download)
	protect("DELETE /api/certificates/{domain}", r.certHandler.Delete)
	notifyHandler := NewNotifyHandler(deps.Notify)
	protect("GET /api/settings", r.settingsHandler.Get)
	protect("PUT /api/settings", r.settingsHandler.Put)
	protect("POST /api/settings/notify/test", notifyHandler.Test)
	if deps.ChinaCIDR != nil {
		protect("GET /api/settings/china-cidr", r.chinaCIDRHandler.Status)
		protect("POST /api/settings/china-cidr/refresh", r.chinaCIDRHandler.Refresh)
	}
	protect("GET /api/logs/access", r.logsHandler.Access)
	protect("GET /api/logs/error", r.logsHandler.Error)
	protect("GET /api/logs/system", r.logsHandler.System)
	protect("GET /api/metrics/history", r.metricsHandler.History)
	protect("GET /api/logs/stream", r.logsHandler.Stream)
	protect("GET /api/system/update/status", r.updateHandler.Status)
	protect("POST /api/system/update/apply", r.updateHandler.Apply)
	protect("GET /api/backup/export", r.backupHandler.Export)
	protect("POST /api/backup/restore", r.backupHandler.Restore)
	protect("GET /api/backup/archives", r.backupHandler.ListArchives)
	protect("POST /api/backup/archives", r.backupHandler.CreateArchive)
	protect("GET /api/backup/archives/{name}", r.backupHandler.DownloadArchive)
	protect("POST /api/backup/archives/{name}/restore", r.backupHandler.RestoreArchive)
	protect("DELETE /api/backup/archives/{name}", r.backupHandler.DeleteArchive)
	protect("GET /api/discovery/scan", r.discoveryHandler.Scan)
	if deps.FRPMulti != nil {
		frpHandler := r.frpmultiHandler
		protect("GET /api/frpmulti/status", frpHandler.Status)
		protect("GET /api/frpmulti/runtime", frpHandler.Runtime)
		protect("PUT /api/frpmulti/runtime/download-proxy", frpHandler.SetDownloadProxy)
		protect("GET /api/frpmulti/releases", frpHandler.Releases)
		protect("GET /api/frpmulti/binaries", frpHandler.Binaries)
		protect("GET /api/frpmulti/download", frpHandler.DownloadStatus)
		protect("POST /api/frpmulti/releases/{version}/download", frpHandler.DownloadRelease)
		protect("POST /api/frpmulti/binaries/{version}/activate", frpHandler.ActivateBinary)
		protect("DELETE /api/frpmulti/binaries/{version}", frpHandler.DeleteBinary)
		protect("GET /api/frpmulti/servers", frpHandler.ListServers)
		protect("GET /api/frpmulti/servers/{id}/detail", frpHandler.ServerDetail)
		protect("GET /api/frpmulti/servers/{id}/diagnostics", frpHandler.ServerDiagnostics)
		protect("GET /api/frpmulti/servers/{id}/frps-config", frpHandler.FRPSServerConfig)
		protect("POST /api/frpmulti/servers/{id}/start", frpHandler.StartServer)
		protect("POST /api/frpmulti/servers/{id}/stop", frpHandler.StopServer)
		protect("POST /api/frpmulti/servers", frpHandler.CreateServer)
		protect("PUT /api/frpmulti/servers/{id}", frpHandler.UpdateServer)
		protect("DELETE /api/frpmulti/servers/{id}", frpHandler.DeleteServer)
		protect("POST /api/frpmulti/servers/{id}/test", frpHandler.TestServer)
		protect("POST /api/frpmulti/servers/{id}/agent/probe", frpHandler.AgentProbe)
		protect("POST /api/frpmulti/servers/{id}/agent/install", frpHandler.AgentInstall)
		protect("POST /api/frpmulti/servers/{id}/agent/uninstall", frpHandler.AgentUninstall)
		protect("POST /api/frpmulti/servers/{id}/agent/ssh-diagnose", frpHandler.AgentSSHDiagnose)
		protect("POST /api/frpmulti/servers/{id}/agent/test", frpHandler.AgentTest)
		protect("POST /api/frpmulti/servers/{id}/agent/tunnel/restart", frpHandler.AgentTunnelRestart)
		protect("POST /api/frpmulti/servers/{id}/agent/transport", frpHandler.SetAgentTransport)
		protect("GET /api/frpmulti/servers/{id}/agent/transport", frpHandler.AgentTransportStatus)
		protect("POST /api/frpmulti/servers/{id}/agent/transport/rotate", frpHandler.AgentTransportRotateCert)
		protect("GET /api/frpmulti/servers/{id}/agent/firewall", frpHandler.AgentFirewallStatus)
		protect("POST /api/frpmulti/servers/{id}/agent/firewall/allow", frpHandler.AgentFirewallAllow)
		protect("GET /api/frpmulti/servers/{id}/agent/status", frpHandler.AgentStatus)
		protect("GET /api/frpmulti/servers/{id}/agent/routes", frpHandler.AgentRoutes)
		protect("GET /api/frpmulti/servers/{id}/agent/routes/health", frpHandler.AgentRouteHealth)
		protect("GET /api/frpmulti/routes/summary", frpHandler.AgentRouteHealthSummary)
		protect("GET /api/frpmulti/routes/uptime", frpHandler.RouteUptime)
		protect("GET /api/frpmulti/servers/{id}/agent/routes/drift", frpHandler.AgentRouteDrift)
		protect("POST /api/frpmulti/servers/{id}/agent/routes/redeploy", frpHandler.AgentRedeployRoutes)
		protect("GET /api/frpmulti/servers/{id}/agent/metrics", frpHandler.AgentMetrics)
		protect("GET /api/frpmulti/servers/{id}/agent/logs/nginx", frpHandler.AgentNginxLogs)
		protect("GET /api/frpmulti/servers/{id}/agent/frps-config", frpHandler.AgentFRPSConfig)
		protect("GET /api/frpmulti/servers/{id}/agent/certs", frpHandler.AgentCerts)
		protect("POST /api/frpmulti/servers/{id}/agent/nginx/reload", frpHandler.AgentNginxReload)
		protect("POST /api/frpmulti/servers/{id}/agent/nginx/install", frpHandler.AgentInstallNginx)
		protect("GET /api/frpmulti/servers/{id}/agent/routes/{proxyID}/conf", frpHandler.AgentRouteConf)
		protect("GET /api/frpmulti/servers/{id}/agent/routes/{proxyID}/versions", frpHandler.AgentRouteVersions)
		protect("GET /api/frpmulti/servers/{id}/agent/routes/{proxyID}/versions/{name}", frpHandler.AgentRouteVersion)
		protect("POST /api/frpmulti/servers/{id}/agent/routes/{proxyID}/versions/{name}/rollback", frpHandler.AgentRollbackRouteVersion)
		protect("GET /api/frpmulti/servers/{id}/agent/route-candidates", frpHandler.AgentRouteCandidates)
		protect("POST /api/frpmulti/servers/{id}/agent/routes/{proxyID}", frpHandler.DeployAgentRoute)
		protect("DELETE /api/frpmulti/servers/{id}/agent/routes/{proxyID}", frpHandler.RemoveAgentRoute)
		protect("POST /api/frpmulti/servers/{id}/agent/frps-install", frpHandler.InstallFRPSOnAgent)
		protect("POST /api/frpmulti/servers/{id}/agent/frps-action", frpHandler.AgentFRPSAction)
		protect("POST /api/frpmulti/servers/{id}/agent/frps-config", frpHandler.PushFRPSConfigToAgent)
		protect("GET /api/frpmulti/proxies", frpHandler.ListProxies)
		protect("GET /api/frpmulti/proxies/traffic", frpHandler.ListProxiesTraffic)
		protect("POST /api/frpmulti/proxies", frpHandler.CreateProxy)
		protect("PUT /api/frpmulti/proxies/{id}", frpHandler.UpdateProxy)
		protect("DELETE /api/frpmulti/proxies/{id}", frpHandler.DeleteProxy)
		protect("GET /api/frpmulti/proxies/{id}/logs", frpHandler.ProxyLogs)
		protect("POST /api/frpmulti/start", frpHandler.Start)
		protect("POST /api/frpmulti/stop", frpHandler.Stop)
		protect("POST /api/frpmulti/reload", frpHandler.Reload)
		protect("GET /api/frpmulti/logs", frpHandler.Logs)
		protect("GET /api/frpmulti/config/export", frpHandler.ExportConfig)
	}

	if deps.Cloudflare != nil {
		cf := r.cloudflareHandler
		protect("GET /api/cloudflare/tunnels", cf.List)
		protect("POST /api/cloudflare/tunnels", cf.Create)
		protect("GET /api/cloudflare/tunnels/{id}", cf.Get)
		protect("PUT /api/cloudflare/tunnels/{id}", cf.Update)
		protect("DELETE /api/cloudflare/tunnels/{id}", cf.Delete)
		protect("POST /api/cloudflare/tunnels/{id}/start", cf.Start)
		protect("POST /api/cloudflare/tunnels/{id}/stop", cf.Stop)
		protect("POST /api/cloudflare/tunnels/{id}/restart", cf.Restart)
		protect("POST /api/cloudflare/tunnels/{id}/credentials/refresh", cf.RefreshCredentials)
		protect("GET /api/cloudflare/tunnels/{id}/status", cf.Status)
		protect("GET /api/cloudflare/tunnels/{id}/logs", cf.Logs)
		protect("GET /api/cloudflare/tunnels/{id}/config", cf.Config)
		protect("GET /api/cloudflare/tunnels/{id}/routes", cf.Routes)
		protect("PUT /api/cloudflare/tunnels/{id}/routes", cf.ReplaceRoutes)
		protect("GET /api/cloudflare/tunnels/{id}/drift", cf.Drift)
		protect("POST /api/cloudflare/tunnels/{id}/sync", cf.SyncRoutes)
		protect("GET /api/cloudflare/tunnels/{id}/diagnostics", cf.Diagnostics)
		protect("GET /api/cloudflare/binary", cf.BinaryInfo)
		protect("POST /api/cloudflare/binary/download", cf.DownloadBinary)
		protect("GET /api/cloudflare/binary/latest", cf.LatestBinary)
		protect("POST /api/cloudflare/binary/rollback", cf.RollbackBinary)
		protect("GET /api/cloudflare/settings", cf.AppSettings)
		protect("PUT /api/cloudflare/settings", cf.SaveAppSettings)
		protect("POST /api/cloudflare/settings/test", cf.TestToken)
		protect("POST /api/cloudflare/enabled", cf.SetEnabled)
		protect("GET /api/cloudflare/ingress-candidates", cf.IngressCandidates)
		protect("GET /api/cloudflare/zones", cf.Zones)
		protect("GET /api/cloudflare/access-status", cf.AccessStatus)
		protect("GET /api/cloudflare/preflight", cf.Preflight)
		protect("POST /api/cloudflare/routes/test", cf.TestRoute)
	}

	if deps.StaticFS != nil {
		mux.Handle("/", r.spaHandler())
	}

	for _, spec := range specs {
		if spec.Public {
			mux.HandleFunc(spec.Pattern, spec.Handler)
			continue
		}
		mux.Handle(spec.Pattern, SessionMiddleware(deps.Auth)(RequireAuth(deps.Auth)(spec.Handler)))
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return loggingMiddleware(logger, SessionMiddleware(deps.Auth)(mux)), specs
}

type Router struct {
	authHandler       *AuthHandler
	proxyHandler      *ProxyHandler
	nginxHandler      *NginxHandler
	statusHandler     *StatusHandler
	ddnsHandler       *DDNSHandler
	certHandler       *CertHandler
	settingsHandler   *SettingsHandler
	chinaCIDRHandler  *ChinaCIDRHandler
	logsHandler       *LogsHandler
	backupHandler     *BackupHandler
	statusPageHandler *StatusPageHandler
	metricsHandler    *MetricsHandler
	discoveryHandler  *DiscoveryHandler
	frpmultiHandler   *FRPMultiHandler
	cloudflareHandler *CloudflareHandler
	updateHandler     *UpdateHandler
	authSvc           *auth.Service
	staticFS          fs.FS
}

func (r *Router) spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(r.staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			http.NotFound(w, req)
			return
		}

		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(r.staticFS, path); err != nil {
			// SPA fallback: unknown routes serve index.html
			req.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, req)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") &&
			!strings.Contains(r.URL.Path, "/logs/stream") &&
			!strings.Contains(r.URL.Path, "/certificates/jobs/") &&
			rec.status >= 400 {
			level := slog.LevelWarn
			msg := "api request completed with client error"
			if rec.status >= 500 {
				level = slog.LevelError
				msg = "api request failed"
			}
			logger.Log(r.Context(), level, msg,
				"module", "HTTP",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
	})
}
