package frp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/havline/havline/internal/certificate"
	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/fsutil"
	"github.com/havline/havline/internal/secret"
)

// maskedToken 是敏感凭据的统一回显掩码：API 永不返回明文；保存时收到掩码一律视为保留旧值
// （对齐官方网关模式的「脱敏令牌闭环」，防止掩码被当新密钥覆盖真实凭据）。
const maskedToken = "********"

type Service struct {
	cfg       config.Config
	store     *Store
	secretBox *secret.Box
	manager   *Manager
	logger    *slog.Logger
	certStore *certificate.Store // 用于 HTTPS 规则部署时把 Havline 本地证书推送到公网 agent

	managersMu sync.RWMutex
	managers   map[int64]*Manager

	// 穿透流量速率采样缓存：key 为规则 ID,用于在服务端计算两次采集之间的速率
	trafficMu     sync.Mutex
	trafficClient *frpsTrafficClient
	trafficLast   map[int64]trafficSample

	downloadMu   sync.RWMutex
	downloadTask DownloadTask
	// baseCtx 是应用级 ctx：后台下载随之取消（之前用 context.Background()，App 关闭后仍在跑）
	baseCtx    context.Context
	locationMu sync.Mutex
	locations  map[string]locationCache

	// agentClients 缓存每台服务端的 agent HTTP 客户端（含连接池），按 URL+Token 指纹失效
	agentMu      sync.Mutex
	agentClients map[int64]cachedAgentClient

	tunnelMu sync.Mutex
	tunnels  map[int64]*agentTunnel

	// 诊断短 TTL 缓存与并发上限，避免列表页轮询叠加外部探测
	diagMu       sync.Mutex
	diagCache    map[int64]diagnosticsCacheEntry
	diagInFlight int

	// applyMu 保证 FRP 配置文件写入与批量启动/重载的原子边界（23.5）:
	// SQLite 事务只保护关系数据，不保护 frpc-server-*.toml 与 frpc 进程。
	applyMu sync.Mutex
}

func NewService(cfg config.Config, store *Store, secretBox *secret.Box, logger *slog.Logger, certStore *certificate.Store) *Service {
	bin := cfg.FrpcBin
	if active, err := readActiveBinary(cfg.FrpDir()); err == nil && active != "" {
		bin = active
	}
	service := &Service{
		cfg:       cfg,
		store:     store,
		secretBox: secretBox,
		manager:   NewManager(bin, cfg.FrpConfigPath(), cfg.FrpLogPath(), logger),
		managers:  map[int64]*Manager{},
		logger:    logger.With("module", "FRP"),
		certStore: certStore,
		downloadTask: DownloadTask{
			Status: "idle",
		},
		locations:     map[string]locationCache{},
		trafficClient: newFRPSTrafficClient(),
		trafficLast:   map[int64]trafficSample{},
		diagCache:     map[int64]diagnosticsCacheEntry{},
		tunnels:       map[int64]*agentTunnel{},
	}
	return service
}

func (s *Service) Runtime(ctx context.Context) RuntimeInfo {
	s.manager.RefreshVersion(ctx)
	bin, _ := s.manager.BinaryPath()
	info := RuntimeInfo{
		Platform: runtimePlatform(), ConfiguredBin: s.cfg.FrpcBin, ActiveBin: bin,
		ConfigPath: s.cfg.FrpConfigPath(),
	}
	servers, _ := s.store.ListServers(ctx)
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		info.ConfigPaths = append(info.ConfigPaths, s.serverConfigPath(server.ID))
		if fileExists(s.serverConfigPath(server.ID)) {
			info.ConfigExists = true
		}
	}
	if len(info.ConfigPaths) == 0 {
		info.ConfigExists = fileExists(info.ConfigPath)
	}
	info.DetectedVersion = s.manager.Status().Version
	info.ActiveVersion = versionFromBinaryPath(s.cfg.FrpDir(), bin)
	info.DownloadProxy, _ = readDownloadProxy(s.cfg.FrpDir())
	return info
}

func (s *Service) ListReleases(ctx context.Context) ([]Release, error) {
	proxy, err := readDownloadProxy(s.cfg.FrpDir())
	if err != nil {
		return nil, err
	}
	return fetchOfficialReleases(ctx, proxy)
}

func (s *Service) ListBinaries() ([]Binary, error) {
	active, _ := s.manager.BinaryPath()
	return listBinaries(s.cfg.FrpDir(), active)
}

func (s *Service) ServerDetail(ctx context.Context, id int64) (ServerDetail, error) {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return ServerDetail{}, err
	}
	s.enrichTLSStatus(&server)
	s.enrichServerRuntime(ctx, &server)
	status, err := s.store.ServerDiagnosticDetail(ctx, id)
	if err != nil {
		return ServerDetail{}, err
	}
	proxies, err := s.store.ListProxiesByServer(ctx, id)
	if err != nil {
		return ServerDetail{}, err
	}
	return ServerDetail{Server: server, Status: status, Proxies: proxies}, nil
}

func diagnoseDialError(err error) string {
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "access permissions") {
		return "本机网络策略阻止连接：" + message
	}
	return "连接失败：" + message
}

func (s *Service) DeleteBinary(version string) error {
	path, err := binaryPathForVersion(s.cfg.FrpDir(), version)
	if err != nil {
		return err
	}
	active, _ := s.manager.BinaryPath()
	if filepath.Clean(path) == filepath.Clean(active) {
		return fmt.Errorf("不能删除当前正在使用的 FRP 客户端")
	}
	return removeBinaryVersion(s.cfg.FrpDir(), version, path)
}

func (s *Service) StartDownload(version string) (DownloadTask, error) {
	if !validVersion(version) {
		return DownloadTask{}, fmt.Errorf("FRP 版本格式无效")
	}
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	if downloadInProgress(s.downloadTask.Status) {
		return DownloadTask{}, fmt.Errorf("FRP %s 正在下载，请等待当前任务结束", s.downloadTask.Version)
	}
	s.downloadTask = DownloadTask{Version: version, Status: "queued", Message: "正在准备下载", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	go s.runDownload(version)
	return s.downloadTask, nil
}

// SetBaseContext 注入应用级 ctx，供后台下载继承；由 app 启动时调用。
func (s *Service) SetBaseContext(ctx context.Context) {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	s.baseCtx = ctx
}

func (s *Service) DownloadStatus() DownloadTask {
	s.downloadMu.RLock()
	defer s.downloadMu.RUnlock()
	return s.downloadTask
}

func (s *Service) runDownload(version string) {
	s.downloadMu.RLock()
	base := s.baseCtx
	s.downloadMu.RUnlock()
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 10*time.Minute)
	defer cancel()
	binary, err := s.downloadRelease(ctx, version, s.updateDownload)
	if err != nil {
		s.updateDownload("failed", "下载失败", 0, 0, err.Error())
		return
	}
	s.updateDownload("completed", "下载、校验和解压完成", binary.Size, binary.Size, "")
}

func (s *Service) downloadRelease(ctx context.Context, version string, report downloadReporter) (Binary, error) {
	report("queued", "正在获取官方版本信息", 0, 0, "")
	releases, err := s.ListReleases(ctx)
	if err != nil {
		return Binary{}, err
	}
	release, ok := findRelease(releases, version)
	if !ok {
		return Binary{}, fmt.Errorf("未找到适用于当前系统的官方 FRP 版本")
	}
	proxy, err := readDownloadProxy(s.cfg.FrpDir())
	if err != nil {
		return Binary{}, err
	}
	path, err := downloadOfficialBinary(ctx, s.cfg.FrpDir(), release, proxy, report)
	if err != nil {
		return Binary{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Binary{}, err
	}
	return Binary{Version: release.Version, Path: path, Size: info.Size(), InstalledAt: info.ModTime().UTC().Format(time.RFC3339)}, nil
}

func (s *Service) updateDownload(status, message string, downloaded, total int64, failure string) {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	s.downloadTask.Status = status
	s.downloadTask.Message = message
	if downloaded > 0 {
		s.downloadTask.Downloaded = downloaded
	}
	if total > 0 {
		s.downloadTask.Total = total
	}
	s.downloadTask.Error = failure
	s.downloadTask.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func downloadInProgress(status string) bool {
	return status == "queued" || status == "downloading" || status == "verifying" || status == "extracting"
}

func (s *Service) SetDownloadProxy(ctx context.Context, proxy string) (RuntimeInfo, error) {
	if err := writeDownloadProxy(s.cfg.FrpDir(), proxy); err != nil {
		return RuntimeInfo{}, err
	}
	return s.Runtime(ctx), nil
}

func (s *Service) ActivateBinary(ctx context.Context, version string) (RuntimeStatus, error) {
	path, err := binaryPathForVersion(s.cfg.FrpDir(), version)
	if err != nil {
		return RuntimeStatus{}, err
	}
	old, _ := s.manager.BinaryPath()
	s.setManagerBinary(path)
	if err := s.verifyActiveBinary(ctx); err != nil {
		s.setManagerBinary(old)
		return RuntimeStatus{}, err
	}
	wasRunning := map[int64]bool{}
	for id, manager := range s.managerSnapshot() {
		wasRunning[id] = manager.Status().Status == "running"
		if wasRunning[id] {
			if err := manager.Stop(ctx); err != nil {
				s.setManagerBinary(old)
				return RuntimeStatus{}, err
			}
		}
	}
	if err := writeActiveBinary(s.cfg.FrpDir(), path); err != nil {
		s.setManagerBinary(old)
		return RuntimeStatus{}, err
	}
	for id, running := range wasRunning {
		if running {
			if err := s.managerForServer(id).Start(); err != nil {
				return RuntimeStatus{}, err
			}
		}
	}
	s.manager.RefreshVersion(ctx)
	return s.Status(ctx), nil
}

func (s *Service) verifyActiveBinary(ctx context.Context) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	verified := false
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		verified = true
		manager := s.managerForServer(server.ID)
		if !fileExists(s.serverConfigPath(server.ID)) {
			if err := manager.VerifyBinary(ctx); err != nil {
				return err
			}
			continue
		}
		if err := manager.Verify(ctx, s.serverConfigPath(server.ID)); err != nil {
			return err
		}
	}
	if !verified {
		return s.manager.VerifyBinary(ctx)
	}
	return nil
}

func (s *Service) EnsureDirs() error {
	return os.MkdirAll(s.cfg.FrpDir(), 0o700)
}

func (s *Service) ListServers(ctx context.Context) ([]Server, error) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	for index := range servers {
		s.enrichTLSStatus(&servers[index])
		s.enrichServerRuntime(ctx, &servers[index])
	}
	return servers, nil
}

func (s *Service) enrichServerRuntime(ctx context.Context, server *Server) {
	runtime, err := s.store.ServerRuntimeStatus(ctx, server.ID)
	if err != nil {
		runtime = ServerRuntimeStatus{Status: "error", DesiredState: "error", ProcessState: "error", ConnectionState: "unknown", LastError: "读取启动状态失败"}
	} else {
		runtime = s.observeServerRuntime(ctx, server.ID, runtime)
	}
	if runtime.DesiredState == "" {
		runtime.DesiredState = runtime.Status
	}
	server.BootStatus = runtime.DesiredState
	server.DesiredState = runtime.DesiredState
	server.ProcessState = runtime.ProcessState
	server.ConnectionState = runtime.ConnectionState
	server.StateSource = runtime.StateSource
	server.ProcessPID = runtime.ProcessPID
	server.ExitCode = runtime.ExitCode
	server.LastConnectedAt = runtime.LastConnectedAt
	server.LastDisconnectedAt = runtime.LastDisconnectedAt
	server.LastError = runtime.LastError
	server.LastStartedAt = runtime.LastStartedAt
	if state, lastError, ok := s.tunnelState(server.ID); ok {
		server.AgentTransportState = state
		server.AgentTransportError = lastError
	}
}

func (s *Service) setServerRuntime(ctx context.Context, serverID int64, status, lastError string) {
	runtime := ServerRuntimeStatus{ServerID: serverID, Status: status, DesiredState: status, LastError: lastError}
	if status == "starting" || status == "running" {
		runtime.LastStartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.store.SetServerDesiredState(ctx, runtime); err != nil {
		s.logger.Warn("保存服务端启动状态失败", "server_id", serverID, "error", err.Error())
	}
}

func (s *Service) observeServerRuntime(ctx context.Context, serverID int64, runtime ServerRuntimeStatus) ServerRuntimeStatus {
	manager := s.managerForServer(serverID)
	process := manager.Status()
	processState := process.Status
	if processState == "stopped" && process.LastError != "" {
		processState = "error"
	}
	connectionState, connectionError := inspectConnectionLog(s.serverLogPath(serverID), processState)
	if connectionError != "" && process.LastError == "" {
		process.LastError = connectionError
	}
	if connectionState != runtime.ConnectionState {
		now := time.Now().UTC().Format(time.RFC3339)
		if connectionState == "connected" {
			runtime.LastConnectedAt = now
		}
		if connectionState == "disconnected" || connectionState == "error" {
			runtime.LastDisconnectedAt = now
		}
	}
	runtime.ProcessState = processState
	runtime.ConnectionState = connectionState
	runtime.StateSource = "manager+log"
	runtime.ProcessPID = process.PID
	runtime.LastError = process.LastError
	if connectionError != "" {
		runtime.LastError = connectionError
	}
	if err := s.store.SetServerObservedState(ctx, runtime); err != nil {
		s.logger.Warn("保存 FRP 实际状态失败", "server_id", serverID, "error", err.Error())
	}
	return runtime
}

func (s *Service) StartServer(ctx context.Context, id int64) (Server, error) {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return Server{}, err
	}
	s.setServerRuntime(ctx, id, "running", "")
	s.enrichTLSStatus(&server)
	s.enrichServerRuntime(ctx, &server)
	return server, nil
}

func (s *Service) StopServer(ctx context.Context, id int64) (Server, error) {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return Server{}, err
	}
	s.setServerRuntime(ctx, id, "stopped", "")
	s.enrichTLSStatus(&server)
	s.enrichServerRuntime(ctx, &server)
	return server, nil
}

func (s *Service) ListProxies(ctx context.Context) ([]Proxy, error) {
	return s.store.ListProxies(ctx)
}

// ListProxiesTraffic 采集各 frps 管理接口的代理流量并按规则 ID 聚合。
// dashboard 地址/端口存于服务端 options,密码复用 Token 列;未配置或不可达的服务端按规则记入 Errors。
func (s *Service) ListProxiesTraffic(ctx context.Context) (ProxiesTraffic, error) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return ProxiesTraffic{}, err
	}
	proxies, err := s.store.ListProxies(ctx)
	if err != nil {
		return ProxiesTraffic{}, err
	}
	result := ProxiesTraffic{Items: map[int64]ProxyTraffic{}, Errors: map[int64]string{}, SampledAt: time.Now().UTC().Format(time.RFC3339)}
	for _, proxy := range proxies {
		if !proxy.Enabled {
			continue
		}
		result.Items[proxy.ID] = ProxyTraffic{}
	}
	for _, server := range servers {
		if server.Options.DashboardAddr == "" || server.Options.DashboardPort == 0 {
			continue
		}
		var password string
		if server.DashboardHasPwd {
			encoded, err := s.store.DashboardPassword(ctx, server.ID)
			if err != nil {
				continue
			}
			value, err := s.secretBox.Decrypt(encoded)
			if err != nil {
				continue
			}
			password = value
		}
		traffic := s.trafficClient.fetchServerTraffic(ctx, server, password)
		if !traffic.OK {
			// 旧版 frps（<v0.71）没有全量代理列表端点：按本服务端的规则名逐条查询
			names := make([]string, 0, len(proxies))
			for _, proxy := range proxies {
				if proxy.ServerID == server.ID && proxy.Enabled {
					names = append(names, proxy.Name)
				}
			}
			traffic = s.trafficClient.fetchLegacyRouteProxies(ctx, server, password, names)
		}
		if !traffic.OK {
			for _, proxy := range proxies {
				if proxy.ServerID == server.ID && proxy.Enabled {
					result.Errors[proxy.ID] = traffic.Error
				}
			}
			continue
		}
		sampledAt := time.Now()
		for _, proxy := range proxies {
			if proxy.ServerID != server.ID || !proxy.Enabled {
				continue
			}
			current, ok := resolveProxyTraffic(traffic, server, proxy)
			if !ok {
				continue
			}
			result.Items[proxy.ID] = s.trafficWithRate(proxy.ID, current, sampledAt)
		}
	}
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result, nil
}

// resolveProxyTraffic 兼容多种代理名形态：裸规则名、user 前缀名（frps 实际注册名，如 admin.ssssss）、域名索引。
func resolveProxyTraffic(traffic ServerTraffic, server Server, proxy Proxy) (ProxyTraffic, bool) {
	if item, ok := traffic.Proxies[proxy.Name]; ok {
		return item, true
	}
	if user := strings.TrimSpace(server.Options.User); user != "" {
		if item, ok := traffic.Proxies[user+"."+proxy.Name]; ok {
			return item, true
		}
	}
	for _, domain := range proxy.CustomDomains {
		if item, ok := traffic.Proxies["domain:"+strings.ToLower(strings.TrimSpace(domain))]; ok {
			return item, true
		}
	}
	return ProxyTraffic{}, false
}

// trafficWithRate 结合上次采样计算 B/s 均值速率;无上次采样、间隔过短或计数回退（如 frps 重启）时速率为 0。
func (s *Service) trafficWithRate(proxyID int64, current ProxyTraffic, at time.Time) ProxyTraffic {
	s.trafficMu.Lock()
	defer s.trafficMu.Unlock()
	if previous, ok := s.trafficLast[proxyID]; ok {
		elapsed := at.Sub(previous.at).Seconds()
		if elapsed >= 2 && current.TrafficIn >= previous.in && current.TrafficOut >= previous.out {
			current.TrafficInRate = int64(float64(current.TrafficIn-previous.in) / elapsed)
			current.TrafficOutRate = int64(float64(current.TrafficOut-previous.out) / elapsed)
		}
	}
	s.trafficLast[proxyID] = trafficSample{at: at, in: current.TrafficIn, out: current.TrafficOut}
	return current
}

func (s *Service) CreateServer(ctx context.Context, in ServerInput) (Server, error) {
	in = normalizeServerInput(in)
	if err := validateServerInput(in); err != nil {
		return Server{}, err
	}
	if err := s.validateCredentials(in, Server{}); err != nil {
		return Server{}, err
	}
	token, err := s.encryptOptional(in.Token)
	if err != nil {
		return Server{}, err
	}
	agentToken, err := s.encryptOptional(in.AgentToken)
	if err != nil {
		return Server{}, err
	}
	sshSecret, err := s.encryptOptional(in.SSHSecret)
	if err != nil {
		return Server{}, err
	}
	server, err := s.store.CreateServer(ctx, in, token, agentToken, sshSecret)
	if err != nil {
		return Server{}, err
	}
	if err := s.saveOIDCSecret(ctx, server.ID, in); err != nil {
		return Server{}, err
	}
	if err := s.saveDashboardPassword(ctx, server.ID, in); err != nil {
		return Server{}, err
	}
	if err := s.saveTLSAssets(server.ID, in); err != nil {
		return Server{}, err
	}
	server, err = s.store.GetServer(ctx, server.ID)
	if err != nil {
		return Server{}, err
	}
	return server, s.persistConfig(ctx)
}

func (s *Service) UpdateServer(ctx context.Context, id int64, in ServerInput) (Server, error) {
	in = normalizeServerInput(in)
	if err := validateServerInput(in); err != nil {
		return Server{}, err
	}
	existing, err := s.store.GetServer(ctx, id)
	if err != nil {
		return Server{}, err
	}
	// 掩码往返保护：回传掩码视为保留旧 Token，前端原样回填掩码也不会覆盖真实凭据
	updateToken := (in.Token != "" && in.Token != maskedToken) || in.ClearToken
	token := ""
	if in.Token != "" && in.Token != maskedToken {
		token, err = s.secretBox.Encrypt(in.Token)
		if err != nil {
			return Server{}, err
		}
	}
	if err := s.validateCredentials(in, existing); err != nil {
		return Server{}, err
	}
	agentTokenUpdate := (in.AgentToken != "" && in.AgentToken != maskedToken) || in.ClearAgentToken
	agentToken := ""
	if in.AgentToken != "" && in.AgentToken != maskedToken {
		agentToken, err = s.secretBox.Encrypt(in.AgentToken)
		if err != nil {
			return Server{}, err
		}
	}
	sshSecretUpdate := (in.SSHSecret != "" && in.SSHSecret != maskedToken) || in.ClearSSHSecret
	sshSecret := ""
	if in.SSHSecret != "" && in.SSHSecret != maskedToken {
		sshSecret, err = s.secretBox.Encrypt(in.SSHSecret)
		if err != nil {
			return Server{}, err
		}
	}
	server, err := s.store.UpdateServer(ctx, id, in, token, updateToken, agentToken, agentTokenUpdate, sshSecret, sshSecretUpdate)
	if err != nil {
		return Server{}, err
	}
	if err := s.saveOIDCSecret(ctx, id, in); err != nil {
		return Server{}, err
	}
	if err := s.saveDashboardPassword(ctx, id, in); err != nil {
		return Server{}, err
	}
	if err := s.saveTLSAssets(id, in); err != nil {
		return Server{}, err
	}
	server, err = s.store.GetServer(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if !server.Enabled {
		_ = s.managerForServer(id).Stop(ctx)
	}
	if !server.MgmtEnabled || server.AgentTransport != "tunnel" {
		s.stopAgentTunnel(id)
	}
	return server, s.persistConfig(ctx)
}

func (s *Service) DeleteServer(ctx context.Context, id int64) error {
	s.stopAgentTunnel(id)
	if err := s.managerForServer(id).Stop(ctx); err != nil {
		return err
	}
	if err := s.store.DeleteServer(ctx, id); err != nil {
		return err
	}
	_ = os.Remove(s.serverConfigPath(id))
	_ = os.Remove(s.serverLogPath(id))
	s.removeManager(id)
	return s.persistConfig(ctx)
}

func (s *Service) CreateProxy(ctx context.Context, in ProxyInput) (Proxy, error) {
	in = normalizeProxyInput(in)
	if err := validateProxyInput(in); err != nil {
		return Proxy{}, err
	}
	if _, err := s.store.GetServer(ctx, in.ServerID); err != nil {
		return Proxy{}, err
	}
	proxy, err := s.store.CreateProxy(ctx, in)
	if err != nil {
		return Proxy{}, err
	}
	if err := s.persistConfig(ctx); err != nil {
		return Proxy{}, err
	}
	// 公网 Nginx 反代改为在 Agent 页面中手动部署，保存穿透规则不再阻塞于 VPS/Nginx。
	return proxy, nil
}

func (s *Service) UpdateProxy(ctx context.Context, id int64, in ProxyInput) (Proxy, error) {
	in = normalizeProxyInput(in)
	if err := validateProxyInput(in); err != nil {
		return Proxy{}, err
	}
	if _, err := s.store.GetServer(ctx, in.ServerID); err != nil {
		return Proxy{}, err
	}
	proxy, err := s.store.UpdateProxy(ctx, id, in)
	if err != nil {
		return Proxy{}, err
	}
	if err := s.persistConfig(ctx); err != nil {
		return Proxy{}, err
	}
	// 公网 Nginx 反代改为在 Agent 页面中手动部署，保存穿透规则不再阻塞于 VPS/Nginx。
	return proxy, nil
}

func (s *Service) sshInstaller(ctx context.Context, serverID int64) (Server, SSHInstaller, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, SSHInstaller{}, err
	}
	if server.SSHHost == "" || server.SSHUser == "" {
		return Server{}, SSHInstaller{}, fmt.Errorf("请先在服务端表单填写 SSH 地址与用户")
	}
	secret := ""
	if server.SSHConfigured {
		encoded, err := s.store.SSHSecret(ctx, serverID)
		if err != nil || encoded == "" {
			return Server{}, SSHInstaller{}, fmt.Errorf("SSH 凭据未保存")
		}
		secret, err = s.secretBox.Decrypt(encoded)
		if err != nil {
			return Server{}, SSHInstaller{}, err
		}
	}
	installer := SSHInstaller{Host: server.SSHHost, Port: server.SSHPort, User: server.SSHUser, KnownHostsPath: s.cfg.SSHKnownHostsPath()}
	if server.SSHAuth == "password" {
		installer.Password = secret
	} else {
		installer.PrivateKey = secret
	}
	return server, installer, nil
}

func (s *Service) AgentProbe(ctx context.Context, serverID int64) (map[string]any, error) {
	_, installer, err := s.sshInstaller(ctx, serverID)
	if err != nil {
		return nil, err
	}
	return AgentInstaller{SSH: installer}.Probe(ctx)
}

func (s *Service) AgentInstall(ctx context.Context, serverID int64) (Server, error) {
	server, installer, err := s.sshInstaller(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	token, err := AgentInstaller{SSH: installer}.Install(ctx)
	if err != nil {
		return Server{}, err
	}
	agentToken, err := s.secretBox.Encrypt(token)
	if err != nil {
		return Server{}, err
	}
	// 已是 HTTPS 直连时，升级 Agent 不应把用户的传输方式改回隧道。
	if server.AgentTransport == "https-pin" {
		return s.keepAgentHTTPSAfterInstall(ctx, server, token, agentToken)
	}
	localPort, listener, err := s.reserveAgentTunnelPort(ctx, serverID, server.AgentLocalPort)
	if err != nil {
		return Server{}, err
	}
	tunnel := newAgentTunnelWithListener(s.tunnelContext(ctx), serverID, localPort, installer, listener)
	if err := tunnel.Start(); err != nil {
		_ = listener.Close()
		return Server{}, err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", localPort)
	client := NewAgentClient(baseURL, token)
	if err := client.Ping(ctx); err != nil {
		tunnel.Stop()
		rollbackErr := (AgentInstaller{SSH: installer}).RestorePreviousConfig(ctx)
		if rollbackErr != nil {
			return Server{}, fmt.Errorf("Agent 已安装，但 SSH 隧道验证失败: %w；恢复旧监听配置也失败: %v", err, rollbackErr)
		}
		return Server{}, fmt.Errorf("Agent 已安装，但 SSH 隧道验证失败，已恢复旧监听配置: %w", err)
	}
	input := ServerInput{
		Name: server.Name, ServerAddr: server.ServerAddr, ServerPort: server.ServerPort,
		TLS: server.TLS, TLSServerName: server.TLSServerName, Enabled: server.Enabled,
		Options: server.Options, AgentURL: baseURL,
		AgentToken: token, SSHHost: server.SSHHost, SSHPort: server.SSHPort, SSHUser: server.SSHUser,
		SSHAuth: server.SSHAuth, MgmtEnabled: true,
	}
	updated, err := s.store.UpdateServer(ctx, serverID, input, "", false, agentToken, true, "", false)
	if err != nil {
		return Server{}, err
	}
	s.tunnelMu.Lock()
	if existing := s.tunnels[serverID]; existing != nil {
		existing.Stop()
	}
	s.tunnels[serverID] = tunnel
	s.tunnelMu.Unlock()
	if err := s.store.SetAgentTransport(ctx, serverID, baseURL, "tunnel", localPort, "", "127.0.0.1:7700", "connected", ""); err != nil {
		tunnel.Stop()
		_ = (AgentInstaller{SSH: installer}).RestorePreviousConfig(ctx)
		return Server{}, err
	}
	// 旧版升级流程可能残留 havlineagent-https.conf，这里通过隧道顺手关闭，避免 7443 继续暴露。
	_ = client.DisableHTTPS(ctx)
	s.invalidateAgentClient(serverID)
	return updated, nil
}

// keepAgentHTTPSAfterInstall 在 HTTPS 直连模式下完成 Agent 升级：
// 保持 agent_url、pin 与 transport 不变，只更新 Token 密文并重新验证连通。
func (s *Service) keepAgentHTTPSAfterInstall(ctx context.Context, server Server, token, agentToken string) (Server, error) {
	baseURL := agentHTTPSBaseURL(server)
	pins := agentTransportPins(server)
	if len(pins) == 0 {
		return Server{}, fmt.Errorf("Agent 已升级，但 HTTPS 直连缺少证书指纹；请重新切换 HTTPS 直连")
	}
	client := NewAgentClientWithPins(baseURL, token, pins...)
	if err := client.Ping(ctx); err != nil {
		return Server{}, fmt.Errorf("Agent 已升级，但 HTTPS 直连验证失败: %w", err)
	}
	input := ServerInput{
		Name: server.Name, ServerAddr: server.ServerAddr, ServerPort: server.ServerPort,
		TLS: server.TLS, TLSServerName: server.TLSServerName, Enabled: server.Enabled,
		Options: server.Options, AgentURL: baseURL,
		AgentToken: token, SSHHost: server.SSHHost, SSHPort: server.SSHPort, SSHUser: server.SSHUser,
		SSHAuth: server.SSHAuth, MgmtEnabled: true,
	}
	if _, err := s.store.UpdateServer(ctx, server.ID, input, "", false, agentToken, true, "", false); err != nil {
		return Server{}, err
	}
	listenAddr := strings.TrimSpace(server.AgentListenAddr)
	if listenAddr == "" {
		listenAddr = fmt.Sprintf("0.0.0.0:%d", listenPort("", 7443))
	}
	if err := s.store.SetAgentTransport(ctx, server.ID, baseURL, "https-pin", server.AgentLocalPort, server.AgentTLSPin, listenAddr, "connected", ""); err != nil {
		return Server{}, err
	}
	s.invalidateAgentClient(server.ID)
	return s.store.GetServer(ctx, server.ID)
}

func (s *Service) tunnelContext(fallback context.Context) context.Context {
	s.downloadMu.RLock()
	base := s.baseCtx
	s.downloadMu.RUnlock()
	if base != nil {
		return base
	}
	if fallback != nil {
		return fallback
	}
	return context.Background()
}

func (s *Service) reserveAgentTunnelPort(ctx context.Context, ownerID int64, preferred int) (int, net.Listener, error) {
	s.tunnelMu.Lock()
	defer s.tunnelMu.Unlock()
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return 0, nil, err
	}
	used := map[int]bool{}
	for _, server := range servers {
		if server.ID != ownerID && server.AgentLocalPort > 0 {
			used[server.AgentLocalPort] = true
		}
	}
	if preferred > 0 && !used[preferred] {
		if listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred)); err == nil {
			return preferred, listener, nil
		}
	}
	for port := 17700; port <= 18700; port++ {
		if used[port] {
			continue
		}
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		return port, listener, nil
	}
	return 0, nil, fmt.Errorf("SSH 隧道本地端口 17700-18700 已耗尽")
}

func (s *Service) ensureAgentTunnel(ctx context.Context, serverID int64) (*agentTunnel, error) {
	s.tunnelMu.Lock()
	if tunnel := s.tunnels[serverID]; tunnel != nil {
		s.tunnelMu.Unlock()
		return tunnel, nil
	}
	s.tunnelMu.Unlock()

	server, installer, err := s.sshInstaller(ctx, serverID)
	if err != nil {
		return nil, err
	}
	localPort, listener, err := s.reserveAgentTunnelPort(ctx, serverID, server.AgentLocalPort)
	if err != nil {
		return nil, err
	}
	tunnel := newAgentTunnelWithListener(s.tunnelContext(ctx), serverID, localPort, installer, listener)
	if err := tunnel.Start(); err != nil {
		_ = listener.Close()
		return nil, err
	}
	s.tunnelMu.Lock()
	if existing := s.tunnels[serverID]; existing != nil {
		s.tunnelMu.Unlock()
		tunnel.Stop()
		return existing, nil
	}
	s.tunnels[serverID] = tunnel
	s.tunnelMu.Unlock()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", localPort)
	if err := s.store.SetAgentTransport(ctx, serverID, baseURL, "tunnel", localPort, "", "127.0.0.1:7700", "connected", ""); err != nil {
		tunnel.Stop()
		return nil, err
	}
	s.invalidateAgentClient(serverID)
	return tunnel, nil
}

// StartAgentTunnels 在应用启动后恢复所有 tunnel transport。
func (s *Service) StartAgentTunnels(ctx context.Context) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		s.logger.Warn("恢复 Agent 隧道失败", "error", err.Error())
		return
	}
	for _, server := range servers {
		if !server.MgmtEnabled || server.AgentTransport != "tunnel" {
			continue
		}
		if _, err := s.ensureAgentTunnel(ctx, server.ID); err != nil {
			s.logger.Warn("Agent 隧道启动失败", "server_id", server.ID, "error", err.Error())
			continue
		}
		s.cleanupLegacyAgentHTTPS(ctx, server.ID)
	}
}

// cleanupLegacyAgentHTTPS 清理隧道模式下可能残留的 havlineagent-https.conf，避免 7443 继续暴露。
func (s *Service) cleanupLegacyAgentHTTPS(ctx context.Context, serverID int64) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return
	}
	_ = client.DisableHTTPS(ctx)
}

func (s *Service) stopAgentTunnels() {
	s.tunnelMu.Lock()
	tunnels := make([]*agentTunnel, 0, len(s.tunnels))
	for id, tunnel := range s.tunnels {
		tunnels = append(tunnels, tunnel)
		delete(s.tunnels, id)
	}
	s.tunnelMu.Unlock()
	for _, tunnel := range tunnels {
		tunnel.Stop()
	}
}

func (s *Service) stopAgentTunnel(serverID int64) {
	s.tunnelMu.Lock()
	tunnel := s.tunnels[serverID]
	delete(s.tunnels, serverID)
	s.tunnelMu.Unlock()
	if tunnel != nil {
		tunnel.Stop()
	}
}

func (s *Service) tunnelState(serverID int64) (string, string, bool) {
	s.tunnelMu.Lock()
	tunnel := s.tunnels[serverID]
	s.tunnelMu.Unlock()
	if tunnel == nil {
		return "", "", false
	}
	state, lastError := tunnel.State()
	return state, lastError, true
}

func (s *Service) RestartAgentTunnel(ctx context.Context, serverID int64) (Server, error) {
	current, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	s.stopAgentTunnel(serverID)
	if _, err := s.ensureAgentTunnel(ctx, serverID); err != nil {
		_ = s.store.SetAgentTransport(ctx, serverID, current.AgentURL, "tunnel", current.AgentLocalPort, current.AgentTLSPin, current.AgentListenAddr, "failed", err.Error())
		return Server{}, err
	}
	s.cleanupLegacyAgentHTTPS(ctx, serverID)
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	s.enrichServerRuntime(ctx, &server)
	return server, nil
}

// AgentTransportStatus 返回传输方式的详细状态，供前端与诊断使用。
func (s *Service) AgentTransportStatus(ctx context.Context, serverID int64) (map[string]any, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	state := server.AgentTransportState
	lastError := server.AgentTransportError
	if server.AgentTransport == "tunnel" {
		if tunnelState, tunnelError, ok := s.tunnelState(serverID); ok {
			state, lastError = tunnelState, tunnelError
		}
	}
	return map[string]any{
		"transport":      server.AgentTransport,
		"state":          state,
		"agent_url":      server.AgentURL,
		"local_port":     server.AgentLocalPort,
		"listen_addr":    server.AgentListenAddr,
		"tls_pin":        server.AgentTLSPin,
		"tls_not_after":  server.AgentTLSNotAfter,
		"pin_prev_until": server.AgentTLSPinPrevUntil,
		"firewall_state": server.AgentFirewallState,
		"last_error":     lastError,
	}, nil
}

// AgentFirewallStatus 检测 VPS 主机防火墙，并把结果缓存到服务端记录供 UI 展示。
func (s *Service) AgentFirewallStatus(ctx context.Context, serverID int64, port int) (map[string]any, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("端口无效")
	}
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("当前 Agent 连接不可用")
	}
	status, err := client.FirewallStatus(ctx, port)
	if err != nil {
		return nil, err
	}
	_ = s.store.SetAgentFirewallState(ctx, serverID, firewallSummary(status))
	return status, nil
}

// AllowAgentFirewallPort 由用户显式触发，Agent 仅对 ufw / firewalld 执行放行。
func (s *Service) AllowAgentFirewallPort(ctx context.Context, serverID int64, port int) (map[string]any, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("端口无效")
	}
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("当前 Agent 连接不可用")
	}
	result, err := client.AllowFirewallPort(ctx, port)
	if err != nil {
		return nil, err
	}
	_ = s.store.SetAgentFirewallState(ctx, serverID, firewallSummary(result))
	return result, nil
}

func firewallSummary(result map[string]any) string {
	status := result
	if nested, ok := result["status"].(map[string]any); ok {
		status = nested
	}
	tool, _ := status["tool"].(string)
	open, _ := status["port_open"].(bool)
	if tool == "" {
		return ""
	}
	if open {
		return tool + ":已放行"
	}
	return tool + ":未放行"
}

func (s *Service) agentToken(ctx context.Context, serverID int64) (string, error) {
	encoded, err := s.store.AgentToken(ctx, serverID)
	if err != nil || encoded == "" {
		return "", fmt.Errorf("Agent Token 未保存")
	}
	token, err := s.secretBox.Decrypt(encoded)
	if err != nil || token == "" {
		return "", fmt.Errorf("Agent Token 无法解密")
	}
	return token, nil
}

func (s *Service) EnableAgentHTTPS(ctx context.Context, serverID int64, port int) (Server, error) {
	if port < 0 || port > 65535 {
		return Server{}, fmt.Errorf("HTTPS 端口无效")
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	if server.AgentTransport == "http" {
		return Server{}, fmt.Errorf("请先通过「升级 Agent」迁移到 SSH 隧道，再切换 HTTPS 直连")
	}
	currentClient, ok := s.agentClient(ctx, serverID)
	if !ok {
		return Server{}, fmt.Errorf("当前 Agent 连接不可用")
	}
	token, err := s.agentToken(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	cert, err := generateAgentCertificate(server.SSHHost)
	if err != nil {
		return Server{}, err
	}
	result, err := currentClient.ConfigureHTTPS(ctx, cert.CertPEM, cert.KeyPEM, port)
	if err != nil {
		return Server{}, err
	}
	actualPort := resultPort(result, port)
	if actualPort <= 0 {
		return Server{}, fmt.Errorf("Agent 未返回可用的 HTTPS 端口")
	}
	baseURL := fmt.Sprintf("https://%s:%d", server.SSHHost, actualPort)
	pinned := NewAgentClientWithPin(baseURL, token, cert.Pin)
	if err := pinned.Ping(ctx); err != nil {
		_ = currentClient.DisableHTTPS(ctx)
		if result["self_check"] == "ok" {
			return Server{}, fmt.Errorf("HTTPS 直连验证失败：Agent 本机 %d 端口自检通过，但公网 %s 返回的证书不一致，请检查该端口是否被其他服务、端口转发或防火墙接管: %w", actualPort, baseURL, err)
		}
		return Server{}, fmt.Errorf("HTTPS 直连验证失败: %w", err)
	}
	if err := s.store.SetAgentTransport(ctx, serverID, baseURL, "https-pin", server.AgentLocalPort, cert.Pin, fmt.Sprintf("0.0.0.0:%d", actualPort), "connected", ""); err != nil {
		return Server{}, err
	}
	if err := s.store.SetAgentTLSPinRotation(ctx, serverID, cert.Pin, "", "", cert.NotAfter.UTC().Format(time.RFC3339)); err != nil {
		return Server{}, err
	}
	s.invalidateAgentClient(serverID)
	s.stopAgentTunnel(serverID)
	updated, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	return updated, nil
}

const (
	agentCertRenewBefore = 30 * 24 * time.Hour
	agentCertPinGrace    = 7 * 24 * time.Hour
)

// StartAgentCertificateRenewal 后台巡检 HTTPS 直连证书，剩余不足 30 天时自动轮换。
func (s *Service) StartAgentCertificateRenewal(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	s.renewAgentCertificates(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.renewAgentCertificates(ctx)
		}
	}
}

func (s *Service) renewAgentCertificates(ctx context.Context) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		s.logger.Warn("巡检 Agent 证书失败", "error", err.Error())
		return
	}
	for _, server := range servers {
		if !server.MgmtEnabled || server.AgentTransport != "https-pin" {
			continue
		}
		notAfter, ok := s.agentCertificateExpiry(ctx, server)
		if !ok || time.Until(notAfter) > agentCertRenewBefore {
			continue
		}
		if _, err := s.RotateAgentCertificate(ctx, server.ID); err != nil {
			s.logger.Warn("Agent 证书自动轮换失败", "server_id", server.ID, "error", err.Error())
		}
	}
}

// agentCertificateExpiry 优先使用数据库缓存；旧记录缺少到期时间时从 Agent 补齐。
func (s *Service) agentCertificateExpiry(ctx context.Context, server Server) (time.Time, bool) {
	if raw := strings.TrimSpace(server.AgentTLSNotAfter); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed, true
		}
	}
	client, ok := s.agentClient(ctx, server.ID)
	if !ok {
		return time.Time{}, false
	}
	state, err := client.HTTPSState(ctx)
	if err != nil {
		return time.Time{}, false
	}
	raw, _ := state["not_after"].(string)
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, false
	}
	_ = s.store.SetAgentTLSNotAfter(ctx, server.ID, parsed.UTC().Format(time.RFC3339))
	return parsed, true
}

// RotateAgentCertificate 安装新证书；过渡期内客户端同时接受新 pin 与旧 pin。
func (s *Service) RotateAgentCertificate(ctx context.Context, serverID int64) (Server, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	if server.AgentTransport != "https-pin" {
		return Server{}, fmt.Errorf("仅 HTTPS 直连模式支持证书轮换")
	}
	token, err := s.agentToken(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	cert, err := generateAgentCertificate(server.SSHHost)
	if err != nil {
		return Server{}, err
	}
	oldPin := strings.TrimSpace(server.AgentTLSPin)
	port := listenPort(server.AgentListenAddr, 7443)
	configureClient := NewAgentClientWithPins(agentHTTPSBaseURL(server), token, cert.Pin, oldPin)
	result, err := configureClient.ConfigureHTTPS(ctx, cert.CertPEM, cert.KeyPEM, port)
	if err != nil {
		return Server{}, err
	}
	actualPort := resultPort(result, port)
	baseURL := fmt.Sprintf("https://%s:%d", server.SSHHost, actualPort)
	graceUntil := time.Now().Add(agentCertPinGrace).UTC().Format(time.RFC3339)
	notAfter := cert.NotAfter.UTC().Format(time.RFC3339)
	if err := s.store.SetAgentTransport(ctx, serverID, baseURL, "https-pin", server.AgentLocalPort, cert.Pin, fmt.Sprintf("0.0.0.0:%d", actualPort), "connected", ""); err != nil {
		return Server{}, err
	}
	if err := s.store.SetAgentTLSPinRotation(ctx, serverID, cert.Pin, oldPin, graceUntil, notAfter); err != nil {
		return Server{}, err
	}
	s.invalidateAgentClient(serverID)
	dual := NewAgentClientWithPins(baseURL, token, cert.Pin, oldPin)
	if err := dual.Ping(ctx); err != nil {
		return Server{}, fmt.Errorf("证书轮换后公网校验失败（旧 pin 在过渡期仍可用）: %w", err)
	}
	return s.store.GetServer(ctx, serverID)
}

func resultPort(result map[string]any, fallback int) int {
	if raw, ok := result["port"].(float64); ok && raw > 0 && raw <= 65535 {
		return int(raw)
	}
	return fallback
}

func listenPort(listenAddr string, fallback int) int {
	if _, rawPort, err := net.SplitHostPort(strings.TrimSpace(listenAddr)); err == nil {
		if parsed, err := strconv.Atoi(rawPort); err == nil && parsed > 0 && parsed <= 65535 {
			return parsed
		}
	}
	return fallback
}

func (s *Service) SwitchAgentToTunnel(ctx context.Context, serverID int64) (Server, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	if server.AgentTransport == "https-pin" {
		token, err := s.agentToken(ctx, serverID)
		if err != nil {
			return Server{}, err
		}
		client := NewAgentClientWithPin(agentHTTPSBaseURL(server), token, server.AgentTLSPin)
		if err := client.DisableHTTPS(ctx); err != nil {
			return Server{}, fmt.Errorf("关闭 HTTPS 直连失败: %w", err)
		}
	}
	if _, err := s.ensureAgentTunnel(ctx, serverID); err != nil {
		return Server{}, err
	}
	updated, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return Server{}, err
	}
	s.enrichServerRuntime(ctx, &updated)
	return updated, nil
}

func (s *Service) AgentUninstall(ctx context.Context, serverID int64, keepData bool) (string, error) {
	_, installer, err := s.sshInstaller(ctx, serverID)
	if err != nil {
		return "", err
	}
	output, err := AgentInstaller{SSH: installer}.Uninstall(ctx, keepData)
	if err == nil {
		s.stopAgentTunnel(serverID)
		_ = s.store.SetAgentTransport(ctx, serverID, "", "tunnel", 0, "", "", "stopped", "")
	}
	return output, err
}

func (s *Service) AgentSSHDiagnose(ctx context.Context, serverID int64) (string, error) {
	_, installer, err := s.sshInstaller(ctx, serverID)
	if err != nil {
		return "", err
	}
	return AgentInstaller{SSH: installer}.Diagnose(ctx)
}

func (s *Service) AgentTest(ctx context.Context, serverID int64) error {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	return client.Ping(ctx)
}

// AgentClient 导出指定服务端的 agent 客户端（供 handler 直取明细端点）；不可用返回 false。
func (s *Service) AgentClient(ctx context.Context, serverID int64) (*AgentClient, bool) {
	return s.agentClient(ctx, serverID)
}

func (s *Service) AgentStatus(ctx context.Context, serverID int64) (map[string]any, error) {
	statusCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	server, err := s.store.GetServer(statusCtx, serverID)
	if err != nil {
		return nil, err
	}
	if !server.AgentConfigured {
		return map[string]any{
			"configured":   false,
			"mgmt_enabled": false,
			"available":    false,
			"message":      "公网 agent 未配置或 Token 未保存",
		}, nil
	}
	if !server.MgmtEnabled {
		return map[string]any{
			"configured":   true,
			"mgmt_enabled": false,
			"available":    false,
			"message":      "Agent 已配置，但未启用日常管理",
		}, nil
	}
	if server.AgentTransport == "tunnel" {
		if state, lastError, ok := s.tunnelState(serverID); ok && (state == "failed" || state == "reconnecting") {
			if strings.TrimSpace(lastError) == "" {
				lastError = state
			}
			return nil, fmt.Errorf("SSH 隧道未连接：%s", lastError)
		}
	}
	client, ok := s.agentClient(statusCtx, serverID)
	if !ok {
		return map[string]any{
			"configured":   false,
			"mgmt_enabled": false,
			"available":    false,
			"message":      "公网 agent 未配置或 Token 未保存",
		}, nil
	}
	status, err := client.Status(statusCtx)
	if err != nil {
		return nil, err
	}
	if status == nil {
		status = map[string]any{}
	}
	status["configured"] = true
	status["mgmt_enabled"] = true
	return status, nil
}

func (s *Service) InstallFRPSOnAgent(ctx context.Context, serverID int64, version, proxy string) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	result, err := client.InstallFRPS(ctx, version, proxy)
	if err != nil {
		return nil, err
	}
	// 安装后把同源 frps.toml 一起下发，保证 systemd 启动即指向正确配置。
	// 失败不能吞掉：否则前端只看到「安装成功」，VPS 上跑的仍是旧配置。
	warning := ""
	if server, err := s.store.GetServer(ctx, serverID); err != nil {
		warning = "frps 已安装，但读取服务端配置失败：" + err.Error()
	} else if content, err := s.buildFRPSServerConfig(ctx, server); err != nil {
		warning = "frps 已安装，但生成 frps.toml 失败：" + err.Error()
	} else if err := client.PushFRPSConfig(ctx, content); err != nil {
		warning = "frps 已安装，但 frps.toml 下发失败：" + err.Error()
	}
	if warning != "" {
		result["config_warning"] = warning
		if list, ok := result["notes"].([]any); ok {
			result["notes"] = append(list, warning)
		}
	}
	return result, nil
}

func (s *Service) AgentFRPSAction(ctx context.Context, serverID int64, action string) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	return client.FRPSAction(ctx, action)
}

func (s *Service) PushFRPSConfigToAgent(ctx context.Context, serverID int64) error {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	// 下发的是 frps 服务端配置（vhost 端口/auth/webServer），不是 frpc 客户端配置
	content, err := s.buildFRPSServerConfig(ctx, server)
	if err != nil {
		return err
	}
	return client.PushFRPSConfig(ctx, content)
}

// agentUpstreamFor 返回该规则反代时的回源地址（frps vhost 端口），端口来自服务端表单自定义，
// 未配置时用默认 8080/8443，与 frps.toml 生成器同源。
func (s *Service) agentUpstreamFor(ctx context.Context, proxy Proxy) (string, error) {
	server, err := s.store.GetServer(ctx, proxy.ServerID)
	if err != nil {
		return "", err
	}
	httpPort := server.Options.VhostHTTPPort
	if httpPort <= 0 {
		httpPort = 8080
	}
	httpsPort := server.Options.VhostHTTPSPort
	if httpsPort <= 0 {
		httpsPort = 8443
	}
	if proxy.Type == "https" {
		return fmt.Sprintf("127.0.0.1:%d", httpsPort), nil
	}
	return fmt.Sprintf("127.0.0.1:%d", httpPort), nil
}

// InstallAgentNginx 触发 VPS 上一键安装 Nginx（需 agent 以 root 运行）。
func (s *Service) InstallAgentNginx(ctx context.Context, serverID int64) (map[string]any, error) {
	client, ok := s.agentClient(ctx, serverID)
	if !ok {
		return nil, fmt.Errorf("该服务端未配置公网 agent（缺少地址或令牌）")
	}
	return client.InstallNginx(ctx)
}

// RouteHealthSummary 公网反代部署健康的汇总（仪表盘用）。
// 健康判定与规则页一致：四段（DNS/证书/隧道/内网服务）全通过才算健康；
// 一个域名可同时命中多段失败，因此各段计数之和可能大于 Routes-Healthy。
type RouteHealthSummary struct {
	Servers     int `json:"servers"`
	Routes      int `json:"routes"`
	Healthy     int `json:"healthy"`
	DNSFailed   int `json:"dns_failed"`
	CertMissing int `json:"cert_missing"`
	TunnelDown  int `json:"tunnel_down"`
	// TunnelUnknown 是「frps 管理接口不可用、隧道状态判不出来」的条数
	TunnelUnknown int      `json:"tunnel_unknown"`
	ServiceDown   int      `json:"service_down"`
	FailedServers []string `json:"failed_servers,omitempty"`
}

// SummarizeRouteHealth 逐台已启用服务端采集公网反代健康后汇总。
// 单台服务端采集失败只记名并继续其它服务端，不阻断整轮汇总。
func (s *Service) SummarizeRouteHealth(ctx context.Context) (RouteHealthSummary, error) {
	servers, err := s.store.ListEnabledServers(ctx)
	if err != nil {
		return RouteHealthSummary{}, err
	}
	var (
		items  []RouteHealthCandidate
		failed []string
	)
	for _, server := range servers {
		candidates, err := s.RouteHealth(ctx, server.ID)
		if err != nil {
			failed = append(failed, server.Name)
			continue
		}
		items = append(items, candidates...)
	}
	return summarizeRouteHealth(len(servers), failed, items), nil
}

func summarizeRouteHealth(servers int, failedServers []string, items []RouteHealthCandidate) RouteHealthSummary {
	summary := RouteHealthSummary{Servers: servers, Routes: len(items), FailedServers: failedServers}
	for _, item := range items {
		if !item.DNSOK {
			summary.DNSFailed++
		}
		if !item.CertOK {
			summary.CertMissing++
		}
		switch {
		case !item.TunnelKnown:
			// frps 管理接口不可用时隧道状态判不出来，不能计入「不通」
			summary.TunnelUnknown++
		case !item.TunnelOK:
			summary.TunnelDown++
		}
		if !item.ServiceOK {
			summary.ServiceDown++
		}
		if item.DNSOK && item.CertOK && item.TunnelOK && item.ServiceOK {
			summary.Healthy++
		}
	}
	return summary
}

// RouteHealthCandidate 公网反代规则的四段健康状态（DNS/证书/隧道/内网服务）。
type RouteHealthCandidate struct {
	ProxyID    int64  `json:"proxy_id"`
	Domain     string `json:"domain"`
	DNSOK      bool   `json:"dns_ok"`
	DNSDetail  string `json:"dns_detail,omitempty"`
	CertOK     bool   `json:"cert_ok"`
	CertDetail string `json:"cert_detail,omitempty"`
	TunnelOK   bool   `json:"tunnel_ok"`
	// TunnelKnown 表示「隧道状态是判得出来的」：frps 管理接口没配或连不上时 TunnelOK 恒为 false，
	// 但那是「无法判定」而不是「不通」——告警与汇总都必须按它区分，否则会误报。
	TunnelKnown  bool   `json:"tunnel_known"`
	TunnelName   string `json:"tunnel_name,omitempty"`
	TunnelDetail string `json:"tunnel_detail,omitempty"`
	ServiceOK    bool   `json:"service_ok"`
}

// RouteHealth 批量返回公网反代规则的四段健康状态：
// DNS 解析是否指向服务端地址、本地证书覆盖情况、frps 隧道注册状态、内网服务探活。
func (s *Service) RouteHealth(ctx context.Context, serverID int64) ([]RouteHealthCandidate, error) {
	candidates, err := s.ListAgentRouteCandidates(ctx, serverID)
	if err != nil {
		return nil, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// 隧道注册名集合：frps dashboard（webServer）可用时取其代理名列表；
	// 调用细节（URL/认证/具体错误）记录到 dashboardDiag，失败时透出给前端。
	tunnelNames := map[string]bool{}
	// 由 dashboard 返回的真实代理名（例如 admin.ssssss），按域名映射用于展示。
	tunnelNameByDomain := map[string]string{}
	dashboardReady := false
	dashboardDiag := ""
	if server.Options.DashboardAddr == "" || server.Options.DashboardPort == 0 {
		dashboardDiag = "frps 管理接口未配置（服务端表单缺少地址或端口）"
	} else {
		encoded, perr := s.store.DashboardPassword(ctx, serverID)
		if perr != nil {
			dashboardDiag = fmt.Sprintf("读取 dashboard 密码失败: %v", perr)
		} else if encoded == "" {
			dashboardDiag = "dashboard 密码未保存（服务端表单留空），frps 已开启认证时无法访问管理接口"
		} else {
			password, derr := s.secretBox.Decrypt(encoded)
			if derr != nil {
				dashboardDiag = fmt.Sprintf("dashboard 密码解密失败: %v", derr)
			} else {
				traffic := s.trafficClient.fetchServerTraffic(ctx, server, password)
				if traffic.OK {
					dashboardReady = true
					for name := range traffic.Proxies {
						tunnelNames[name] = true
					}
					for domain, name := range traffic.ProxyNamesByDomain {
						tunnelNameByDomain[domain] = name
					}
				} else {
					// frp 0.68.0 没有全量代理列表，只支持 /api/proxies/{实际代理名}；
					// 用本地候选规则逐条查询，兼容 user.proxyName（例如 admin.ssssss）。
					names := make([]string, 0, len(candidates))
					for _, candidate := range candidates {
						names = append(names, candidate.Name)
					}
					legacyTraffic := s.trafficClient.fetchLegacyRouteProxies(ctx, server, password, names)
					if legacyTraffic.OK {
						dashboardReady = true
						for name := range legacyTraffic.Proxies {
							if !strings.HasPrefix(name, "domain:") {
								tunnelNames[name] = true
							}
						}
						for domain, name := range legacyTraffic.ProxyNamesByDomain {
							tunnelNameByDomain[domain] = name
						}
					} else {
						dashboardDiag = fmt.Sprintf("frps 管理接口调用失败: %s（旧版逐规则查询失败: %s；Basic Auth 用户 %q）", traffic.Error, legacyTraffic.Error, server.Options.DashboardUser)
					}
				}
			}
		}
	}
	// 本地证书覆盖：域名精确或通配符匹配
	certRecords, cerr := s.certStore.List(ctx)
	if cerr != nil {
		certRecords = nil
	}
	certFor := func(domain string) (*certificate.Record, bool) {
		for i := range certRecords {
			rec := &certRecords[i]
			if rec.Domain == domain {
				return rec, true
			}
			for _, d := range rec.Domains {
				if d == domain || (strings.HasPrefix(d, "*") && strings.HasSuffix(domain, strings.TrimPrefix(d, "*"))) {
					return rec, true
				}
			}
		}
		return nil, false
	}
	resolver := &net.Resolver{}
	out := make([]RouteHealthCandidate, 0, len(candidates))
	for _, proxy := range candidates {
		for _, domain := range proxy.CustomDomains {
			item := RouteHealthCandidate{ProxyID: proxy.ID, Domain: domain}
			// ① DNS：解析域名，与 server_addr（IP 或域名解析后）比对
			ips, derr := resolver.LookupIPAddr(ctx, domain)
			if derr != nil || len(ips) == 0 {
				item.DNSDetail = "DNS 解析失败"
			} else {
				serverIPs, serr := resolver.LookupIPAddr(ctx, server.ServerAddr)
				if serr != nil || len(serverIPs) == 0 {
					item.DNSDetail = "服务端地址无法解析"
				} else {
					matched := false
					for _, a := range ips {
						for _, b := range serverIPs {
							if a.IP.Equal(b.IP) {
								matched = true
							}
						}
					}
					item.DNSOK = matched
					if !matched {
						item.DNSDetail = "未指向服务端地址"
					}
				}
			}
			// ② 证书：本地库覆盖 + 有效期
			if rec, ok := certFor(domain); ok {
				if rec.ExpiresAt != nil {
					days := int(time.Until(*rec.ExpiresAt).Hours() / 24)
					item.CertOK = days > 0
					item.CertDetail = fmt.Sprintf("%d 天后到期", days)
				} else {
					item.CertOK = true
					item.CertDetail = "有效期未知"
				}
			} else {
				item.CertDetail = "本地无证书"
			}
			// ③ 隧道：frps dashboard 的代理名与规则名匹配。frpc 配置了 user 时，
			// frp 会自动给代理名加 "<user>." 前缀（如 admin.ssssss），两种形态都接受。
			item.TunnelName = proxy.Name
			if dashboardReady {
				// dashboard 可用：这一次的隧道状态是「判得出来」的，不管结论是通还是不通
				item.TunnelKnown = true
				// 优先按域名索引匹配（V2 API 返回 spec.http.customDomains，代理名含 user 前缀不可靠），
				// 再回退代理名（裸名或 user.前缀名）
				switch {
				case tunnelNames["domain:"+domain]:
					item.TunnelOK = true
					item.TunnelName = tunnelNameByDomain[strings.ToLower(domain)]
					if item.TunnelName == "" {
						item.TunnelName = proxy.Name
					}
				case tunnelNames[proxy.Name]:
					item.TunnelOK = true
					item.TunnelName = proxy.Name
				case func() bool {
					u := strings.TrimSpace(server.Options.User)
					return u != "" && tunnelNames[u+"."+proxy.Name]
				}():
					item.TunnelOK = true
					item.TunnelName = strings.TrimSpace(server.Options.User) + "." + proxy.Name
				}
				if item.TunnelOK {
					item.TunnelDetail = "frps 已注册: " + item.TunnelName
				} else { // 诊断透出：dashboard 实际返回的代理名列表（供前端悬停排查）
					names := make([]string, 0, len(tunnelNames))
					for name := range tunnelNames {
						name = strings.TrimSpace(name)
						if name != "" && !strings.HasPrefix(name, "domain:") {
							names = append(names, name)
						}
					}
					sort.Strings(names)
					item.TunnelDetail = tunnelMismatchDetail(names, domain)
				}
			} else {
				item.TunnelDetail = dashboardDiag
			}
			// ④ 内网服务探活：Havline 与服务同在内网，直接 TCP 拨测
			conn, serr2 := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", proxy.LocalIP, proxy.LocalPort), 2*time.Second)
			if serr2 == nil {
				_ = conn.Close()
				item.ServiceOK = true
			}
			out = append(out, item)
		}
	}
	return out, nil
}

// tunnelMismatchDetail 区分「frps 一个代理都没有」和「有代理但没绑定当前域名」，
// 避免空列表被拼成 “frps 已注册代理: ；” 这种误导性提示。
func tunnelMismatchDetail(names []string, domain string) string {
	if len(names) == 0 {
		return fmt.Sprintf("frps 当前没有注册任何代理；请确认 frpc 已启动并成功登录，域名 %s 尚未绑定", domain)
	}
	return fmt.Sprintf("frps 已注册代理: %s；但均未绑定域名 %s", strings.Join(names, ", "), domain)
}

// GetProxy 导出单条穿透规则读取（供 handler 校验规则归属与域名）。
func (s *Service) GetProxy(ctx context.Context, id int64) (Proxy, error) {
	return s.store.GetProxy(ctx, id)
}

// ListAgentRouteCandidates 返回指定服务端可手动部署到公网 Nginx 的 HTTP/HTTPS 规则。
func (s *Service) ListAgentRouteCandidates(ctx context.Context, serverID int64) ([]Proxy, error) {
	proxies, err := s.store.ListProxiesByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	candidates := make([]Proxy, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy.Enabled && (proxy.Type == "http" || proxy.Type == "https") && len(proxy.CustomDomains) > 0 {
			candidates = append(candidates, proxy)
		}
	}
	return candidates, nil
}

// DeployAgentRoute 手动把单条穿透规则部署为公网 Nginx 反代。
func (s *Service) DeployAgentRoute(ctx context.Context, proxyID int64) error {
	proxy, err := s.store.GetProxy(ctx, proxyID)
	if err != nil {
		return err
	}
	if !proxy.Enabled || (proxy.Type != "http" && proxy.Type != "https") || len(proxy.CustomDomains) == 0 {
		return fmt.Errorf("只有启用的 HTTP/HTTPS 规则可以部署公网反代")
	}
	client, ok := s.agentClient(ctx, proxy.ServerID)
	if !ok {
		return fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	if proxy.Type == "https" {
		// https 类型隧道是端到端透传：frps 不解密，要求内网服务自己持有证书并使用 TLS。
		// 内网明文 HTTP 服务应改用 http 类型，公网 HTTPS 由 Nginx 挂证书终结（证书自动推送）。
		return fmt.Errorf("https 类型隧道要求内网服务自己提供 TLS（192.168.11.30:8001 这类明文服务无法走通）；" +
			"请把规则类型改为 http（公网 HTTPS 由 Nginx 证书终结），或给内网服务配置证书")
	}
	upstream, uerr := s.agentUpstreamFor(ctx, proxy)
	if uerr != nil {
		return uerr
	}
	// vhost 端口供 agent 回源自检用
	server, serr := s.store.GetServer(ctx, proxy.ServerID)
	if serr != nil {
		return serr
	}
	vhostPort := server.Options.VhostHTTPPort
	if vhostPort <= 0 {
		vhostPort = 8080
	}
	// 仅中国大陆 IP：先把本地已维护的中国 IP 段下发到 agent（geo 变量依赖它）
	if proxy.Options.ChinaOnly {
		data, err := os.ReadFile(s.cfg.ChinaCIDRPath())
		if err != nil || strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("启用「仅中国大陆 IP」前需先在「反向代理」页更新中国 IP 段（本地 china_cidr.conf 不存在或为空）")
		}
		if err := client.SyncChinaCIDR(ctx, string(data)); err != nil {
			return fmt.Errorf("下发中国 IP 段失败: %w", err)
		}
	}
	for _, domain := range proxy.CustomDomains {
		// 证书用于公网 Nginx 的 TLS 终结（443 监听），与 frp 隧道类型解耦：
		// 本地明文 HTTP 服务应选 http 类型规则，浏览器侧 HTTPS 由 Nginx 挂证书承担。
		// https 类型隧道要求内网服务自己做 TLS，否则即使端口对上也无法工作。
		tlsEdge := false
		if certErr := s.pushAgentCertificate(ctx, client, domain); certErr == nil {
			tlsEdge = true
		} else if proxy.Type == "https" {
			return fmt.Errorf("推送证书 %s 失败: %w", domain, certErr)
		}
		result, err := client.SyncRouteSpec(ctx, RouteSpec{
			Domain: domain, Upstream: upstream, TLS: tlsEdge, CertDir: "",
			WebSocket: proxy.Options.WebSocket, VhostPort: vhostPort,
			RedirectHTTPS: proxy.Options.RedirectHTTPS,
			AllowIPs:      proxy.Options.AllowIPs, DenyIPs: proxy.Options.DenyIPs,
			BasicAuthUser:     proxy.Options.BasicAuthUser,
			BasicAuthPassword: proxy.Options.BasicAuthPassword,
			HTTPSDisabled:     proxy.Options.HTTPSDisabled,
			ClientMaxBodySize: proxy.Options.ClientMaxBodySize,
			ProxyReadTimeout:  proxy.Options.ProxyReadTimeout,
			SecurityHeaders:   proxy.Options.SecurityHeaders,
			TLS13Only:         proxy.Options.TLS13Only,
			RateLimitRate:     proxy.Options.RateLimitRate,
			RateLimitBurst:    proxy.Options.RateLimitBurst,
			ConnLimitMax:      proxy.Options.ConnLimitMax,
			ChinaOnly:         proxy.Options.ChinaOnly,
		})
		if err != nil {
			return fmt.Errorf("部署域名 %s 失败: %w", domain, err)
		}
		// 回源自检分级提示，不掩盖灰区：
		if code, ok := result["probe_status"].(float64); ok {
			switch {
			case code == 0:
				return fmt.Errorf("反代已部署，但回源自检失败（连接不上 frps vhost :%d）：检查 frps 是否运行、vhostHTTPPort 是否与表单一致", vhostPort)
			case code == 404:
				return fmt.Errorf("反代已部署，frps 正常但没有找到域名 %s 的隧道注册：请确认内网穿透里存在绑定该域名的 http 规则、类型为 http 且已启用，并已应用配置", domain)
			case code >= 500:
				return fmt.Errorf("反代已部署，隧道已连通但内网服务异常（HTTP %.0f）：请检查内网服务 %s 是否存活", code, upstream)
			}
		}
	}
	return nil
}

// pushAgentCertificate 从 Havline 本地证书存储读取域名对应证书并推送到公网 agent；
// 支持精确域名与通配符（*.example.com）匹配。
func (s *Service) pushAgentCertificate(ctx context.Context, client *AgentClient, domain string) error {
	if s.certStore == nil {
		return fmt.Errorf("证书存储不可用")
	}
	records, err := s.certStore.List(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		matched := record.Domain == domain
		if !matched {
			for _, d := range record.Domains {
				if d == domain || (strings.HasPrefix(d, "*") && strings.HasSuffix(domain, strings.TrimPrefix(d, "*"))) {
					matched = true
					break
				}
			}
		}
		if !matched || record.CertPath == "" || record.KeyPath == "" {
			continue
		}
		certPEM, err := os.ReadFile(record.CertPath)
		if err != nil {
			return fmt.Errorf("读取证书文件失败: %w", err)
		}
		keyPEM, err := os.ReadFile(record.KeyPath)
		if err != nil {
			return fmt.Errorf("读取私钥文件失败: %w", err)
		}
		return client.PushCertificate(ctx, domain, string(certPEM), string(keyPEM))
	}
	return fmt.Errorf("Havline 本地没有域名 %s 的证书；请先在「证书」页申请，或改用 HTTP 类型规则", domain)
}

// RemoveAgentRoute 手动移除单条穿透规则对应的公网 Nginx 反代。
func (s *Service) RemoveAgentRoute(ctx context.Context, proxyID int64) error {
	proxy, err := s.store.GetProxy(ctx, proxyID)
	if err != nil {
		return err
	}
	client, ok := s.agentClient(ctx, proxy.ServerID)
	if !ok {
		return fmt.Errorf("公网 agent 未配置或 Token 未保存")
	}
	for _, domain := range proxy.CustomDomains {
		if err := client.DeleteRoute(ctx, domain); err != nil {
			return fmt.Errorf("移除域名 %s 失败: %w", domain, err)
		}
	}
	return nil
}

func (s *Service) DeleteProxy(ctx context.Context, id int64) error {
	if _, err := s.store.GetProxy(ctx, id); err != nil {
		return err
	}
	if err := s.store.DeleteProxy(ctx, id); err != nil {
		return err
	}
	if err := s.persistConfig(ctx); err != nil {
		return err
	}
	// 公网 Nginx 反代由 Agent 页面手动管理，删除穿透规则不自动访问 VPS。
	return nil
}

// cachedAgentClient 是按 URL+Token 指纹缓存的 agent 客户端；指纹用密文而非明文，避免内存里多存一份 Token。
type cachedAgentClient struct {
	fingerprint string
	client      *AgentClient
}

func (s *Service) invalidateAgentClient(serverID int64) {
	s.agentMu.Lock()
	delete(s.agentClients, serverID)
	s.agentMu.Unlock()
}

func agentHTTPSBaseURL(server Server) string {
	current := strings.TrimRight(strings.TrimSpace(server.AgentURL), "/")
	if strings.HasPrefix(strings.ToLower(current), "https://") {
		return current
	}
	host := strings.TrimSpace(server.SSHHost)
	if host == "" {
		return current
	}
	port := listenPort(server.AgentListenAddr, 7443)
	return fmt.Sprintf("https://%s:%d", host, port)
}

// agentTransportPins 返回当前应接受的证书 pin；轮换过渡期内同时包含旧 pin。
func agentTransportPins(server Server) []string {
	out := make([]string, 0, 2)
	if pin := strings.TrimSpace(server.AgentTLSPin); pin != "" {
		out = append(out, pin)
	}
	prev := strings.TrimSpace(server.AgentTLSPinPrev)
	if prev == "" || !pinGraceActive(server.AgentTLSPinPrevUntil) {
		return out
	}
	return append(out, prev)
}

func pinGraceActive(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	deadline, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return false
	}
	return time.Now().Before(deadline)
}

func (s *Service) agentClient(ctx context.Context, serverID int64) (*AgentClient, bool) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil || !server.MgmtEnabled || !server.AgentConfigured {
		return nil, false
	}
	encoded, err := s.store.AgentToken(ctx, serverID)
	if err != nil || encoded == "" {
		return nil, false
	}
	token, err := s.secretBox.Decrypt(encoded)
	if err != nil || token == "" {
		return nil, false
	}
	baseURL := server.AgentURL
	var pins []string
	if server.AgentTransport == "tunnel" {
		tunnel, err := s.ensureAgentTunnel(ctx, serverID)
		if err != nil {
			return nil, false
		}
		baseURL = fmt.Sprintf("http://127.0.0.1:%d", tunnel.localPort)
	} else if server.AgentTransport == "https-pin" {
		pins = agentTransportPins(server)
		if len(pins) == 0 {
			return nil, false
		}
		baseURL = agentHTTPSBaseURL(server)
		if !strings.HasPrefix(strings.ToLower(baseURL), "https://") {
			return nil, false
		}
		if baseURL != strings.TrimRight(strings.TrimSpace(server.AgentURL), "/") {
			err := s.store.SetAgentTransport(ctx, serverID, baseURL, "https-pin", server.AgentLocalPort,
				server.AgentTLSPin, server.AgentListenAddr, server.AgentTransportState, server.AgentTransportError)
			if err != nil {
				s.logger.Warn("agent https url repair failed", "server_id", serverID, "error", err)
			}
		}
	} else if baseURL == "" {
		return nil, false
	}
	// 复用同一个 http.Client：批量同步 N 条路由时不再每次新建 Transport 与 TLS 握手
	fingerprint := baseURL + "|" + encoded + "|" + strings.Join(pins, ",")
	s.agentMu.Lock()
	defer s.agentMu.Unlock()
	if s.agentClients == nil {
		s.agentClients = make(map[int64]cachedAgentClient)
	}
	if cached, ok := s.agentClients[serverID]; ok && cached.fingerprint == fingerprint {
		return cached.client, true
	}
	var client *AgentClient
	if len(pins) > 0 {
		client = NewAgentClientWithPins(baseURL, token, pins...)
	} else {
		client = NewAgentClient(baseURL, token)
	}
	s.agentClients[serverID] = cachedAgentClient{fingerprint: fingerprint, client: client}
	return client, true
}

func (s *Service) syncAgentRoute(ctx context.Context, proxy Proxy) {
	if !proxy.Enabled || (proxy.Type != "http" && proxy.Type != "https") || len(proxy.CustomDomains) == 0 {
		return
	}
	client, ok := s.agentClient(ctx, proxy.ServerID)
	if !ok {
		return
	}
	upstream, uerr := s.agentUpstreamFor(ctx, proxy)
	if uerr != nil {
		s.logger.Warn("agent route sync skipped", "proxy_id", proxy.ID, "error", uerr)
		return
	}
	for _, domain := range proxy.CustomDomains {
		if err := client.SyncRoute(ctx, domain, upstream, proxy.Type == "https", ""); err != nil {
			s.logger.Warn("agent route sync failed", "proxy_id", proxy.ID, "domain", domain, "error", err)
		}
	}
}

func (s *Service) deleteAgentRoute(ctx context.Context, proxy Proxy) {
	client, ok := s.agentClient(ctx, proxy.ServerID)
	if !ok {
		return
	}
	for _, domain := range proxy.CustomDomains {
		if err := client.DeleteRoute(ctx, domain); err != nil {
			s.logger.Warn("agent route delete failed", "proxy_id", proxy.ID, "domain", domain, "error", err)
		}
	}
}

func (s *Service) Start(ctx context.Context) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	if err := s.persistConfigsLocked(ctx); err != nil {
		return err
	}
	s.manager.RefreshVersion(ctx)
	servers, err := s.store.ListEnabledServers(ctx)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return fmt.Errorf("未启用 FRP 服务端")
	}
	// 23.5：按服务端隔离——某个服务端校验/启动失败不停止其他服务端，
	// 已启动的保持运行，失败原因聚合成一条错误返回。
	var failures []string
	for _, server := range servers {
		manager := s.managerForServer(server.ID)
		if err := manager.Verify(ctx, s.serverConfigPath(server.ID)); err != nil {
			s.recordProcessState(ctx, server.ID, manager, err.Error())
			failures = append(failures, fmt.Sprintf("服务端 %s 校验失败：%v", server.Name, err))
			continue
		}
		if err := manager.Start(); err != nil {
			s.recordProcessState(ctx, server.ID, manager, err.Error())
			failures = append(failures, fmt.Sprintf("服务端 %s 启动失败：%v", server.Name, err))
			continue
		}
		s.recordProcessState(ctx, server.ID, manager, "")
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "；"))
	}
	return nil
}

func (s *Service) AutoStart(ctx context.Context) error {
	servers, err := s.store.ListEnabledServers(ctx)
	if err != nil {
		return err
	}
	shouldStart := false
	for _, server := range servers {
		if server.Options.AutoStart {
			shouldStart = true
			break
		}
	}
	if !shouldStart {
		return nil
	}
	return s.Start(ctx)
}

func (s *Service) Stop(ctx context.Context) error {
	s.stopAgentTunnels()
	_ = s.ReconcileRuntime(ctx)
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	var messages []string
	for _, server := range servers {
		manager := s.managerForServer(server.ID)
		if err := manager.Stop(ctx); err != nil {
			messages = append(messages, fmt.Sprintf("服务端 %d：%v", server.ID, err))
			continue
		}
		s.recordProcessState(ctx, server.ID, manager, "")
	}
	if len(messages) > 0 {
		return fmt.Errorf("%s", strings.Join(messages, "；"))
	}
	return nil
}

func (s *Service) Reload(ctx context.Context) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	if err := s.persistConfigsLocked(ctx); err != nil {
		return err
	}
	servers, err := s.store.ListEnabledServers(ctx)
	if err != nil {
		return err
	}
	// 23.5：按服务端隔离重载，失败不中断其他服务端，聚合报错。
	var failures []string
	for _, server := range servers {
		manager := s.managerForServer(server.ID)
		if err := manager.Verify(ctx, s.serverConfigPath(server.ID)); err != nil {
			s.recordProcessState(ctx, server.ID, manager, err.Error())
			failures = append(failures, fmt.Sprintf("服务端 %s 校验失败：%v", server.Name, err))
			continue
		}
		if err := manager.Restart(ctx); err != nil {
			s.recordProcessState(ctx, server.ID, manager, err.Error())
			failures = append(failures, fmt.Sprintf("服务端 %s 重启失败：%v", server.Name, err))
			continue
		}
		s.recordProcessState(ctx, server.ID, manager, "")
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "；"))
	}
	return nil
}

func (s *Service) recordProcessState(ctx context.Context, serverID int64, manager *Manager, lastError string) {
	status := manager.Status()
	processState := status.Status
	if lastError != "" {
		processState = "error"
	}
	runtime := ServerRuntimeStatus{
		ServerID: serverID, ProcessState: processState, ConnectionState: "unknown",
		StateSource: "manager", ProcessPID: status.PID, LastStartedAt: status.LastStartedAt, LastError: lastError,
	}
	if err := s.store.SetServerObservedState(ctx, runtime); err != nil {
		s.logger.Warn("保存 FRP 进程状态失败", "server_id", serverID, "error", err.Error())
	}
}

func (s *Service) ReconcileRuntime(ctx context.Context) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		runtime, runtimeErr := s.store.ServerRuntimeStatus(ctx, server.ID)
		if runtimeErr != nil || runtime.ProcessPID <= 0 {
			continue
		}
		manager := s.managerForServer(server.ID)
		if manager.Attach(runtime.ProcessPID) {
			runtime.ProcessState = "running"
			runtime.StateSource = "reconciled"
			runtime.LastError = ""
			if err := s.store.SetServerObservedState(ctx, runtime); err != nil {
				return err
			}
			continue
		}
		if processAlive(runtime.ProcessPID) {
			runtime.ProcessState = "unknown"
			runtime.StateSource = "reconcile"
			runtime.LastError = "检测到运行中的进程，但无法确认其属于当前 FRP 配置"
		} else {
			runtime.ProcessState = "stopped"
			runtime.ProcessPID = 0
			runtime.StateSource = "reconcile"
			runtime.LastError = ""
		}
		if err := s.store.SetServerObservedState(ctx, runtime); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Test(ctx context.Context, id int64) error {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return err
	}
	if err := s.persistServerConfigForVerify(ctx, server); err != nil {
		return err
	}
	return s.managerForServer(id).Verify(ctx, s.serverConfigPath(id))
}

// persistServerConfigForVerify 为「测试」路径服务：渲染到临时文件并 verify,通过后才
// 原子替换正式配置；不通过时删除临时文件、正式配置保持原样（23.5 的「临时配置加 verify,
// 只有验证成功才替换正式配置」要求）。
func (s *Service) persistServerConfigForVerify(ctx context.Context, server Server) error {
	content, err := s.renderServerConfig(ctx, server)
	if err != nil {
		return err
	}
	officialPath := s.serverConfigPath(server.ID)
	if err := os.WriteFile(officialPath+".verify", []byte(content), 0o600); err != nil {
		return err
	}
	if err := s.managerForServer(server.ID).Verify(ctx, officialPath+".verify"); err != nil {
		_ = os.Remove(officialPath + ".verify")
		return err
	}
	// 备份失败就不能覆盖唯一副本：否则旧配置没有回退、只剩 .verify 里的新内容
	if err := fsutil.Backup(officialPath); err != nil {
		_ = os.Remove(officialPath + ".verify")
		return fmt.Errorf("备份现有 frps 配置失败，已保留原配置: %w", err)
	}
	return os.Rename(officialPath+".verify", officialPath)
}

func (s *Service) renderServerConfig(ctx context.Context, server Server) (string, error) {
	proxies, err := s.store.ListEnabledProxies(ctx, server.ID)
	if err != nil {
		return "", err
	}
	token, oidcSecret, err := s.loadAuthSecrets(ctx, server)
	if err != nil {
		return "", err
	}
	return renderTOML(renderedConfig{
		Server: server, Token: token, OIDCClientSecret: oidcSecret, LogPath: s.serverLogPath(server.ID),
		TLSCertPath: s.existingTLSAssetPath(server.ID, "cert.pem"), TLSKeyPath: s.existingTLSAssetPath(server.ID, "key.pem"),
		TLSTrustedCAPath: s.existingTLSAssetPath(server.ID, "ca.pem"), Proxies: proxies,
	}), nil
}

func (s *Service) Status(ctx context.Context) RuntimeStatus {
	versionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s.manager.RefreshVersion(versionCtx)
	servers, err := s.store.ListEnabledServers(ctx)
	if err != nil {
		return s.statusWithVersion(s.manager)
	}
	statuses := make([]RuntimeStatus, 0, len(servers))
	for _, server := range servers {
		manager := s.managerForServer(server.ID)
		// 每个服务端各自探测版本（共用同一个 2 秒超时上下文）
		manager.RefreshVersion(versionCtx)
		current := s.statusWithVersion(manager)
		current.Configured = s.serverConfigured(server)
		if proxies, proxyErr := s.store.ListEnabledProxies(ctx, server.ID); proxyErr == nil {
			current.EnabledProxies = len(proxies)
		}
		statuses = append(statuses, current)
	}
	return latestRuntimeStatus(statuses)
}

// statusWithVersion 取服务端运行状态，并在版本探测失败时按二进制目录名推导版本：
// frpc 进程未运行或 --version 探测失败时，RuntimeStatus.Version 因带 omitempty 会被省略，
// 前端（FRP 穿透页状态卡、仪表盘）就只能显示「未检测 / 未知」。
// 这里回落到 <FrpDir>/bin/<版本>/ 的目录约定，与「FRP 应用配置」弹窗的 active_version 同源。
func (s *Service) statusWithVersion(manager *Manager) RuntimeStatus {
	status := manager.Status()
	if status.Version != "" {
		return status
	}
	if bin, err := manager.BinaryPath(); err == nil {
		status.Version = versionFromBinaryPath(s.cfg.FrpDir(), bin)
	}
	return status
}

func (s *Service) ReadLogs(limit int, serverID int64) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	servers, err := s.store.ListServers(context.Background())
	if err != nil {
		return nil, err
	}
	lines := []string{}
	for _, server := range servers {
		// 指定 server_id 时只读该服务端的日志文件；0 表示全部服务端
		if serverID > 0 && server.ID != serverID {
			continue
		}
		data, readErr := os.ReadFile(s.serverLogPath(server.ID))
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.TrimSpace(line) != "" {
				lines = append(lines, fmt.Sprintf("[%s] %s", server.Name, line))
			}
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines, nil
}

// ReadProxyLogs 返回单条穿透规则的日志：读所属服务端的 frpc 日志并按规则名过滤。
func (s *Service) ReadProxyLogs(ctx context.Context, proxyID int64, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	proxy, err := s.store.GetProxy(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	data, readErr := os.ReadFile(s.serverLogPath(proxy.ServerID))
	if os.IsNotExist(readErr) {
		return []string{}, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) != "" && strings.Contains(line, proxy.Name) {
			lines = append(lines, line)
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines, nil
}

func (s *Service) ExportMaskedConfig(ctx context.Context) (string, error) {
	servers, err := s.store.ListEnabledServers(ctx)
	if err != nil {
		return "", err
	}
	if len(servers) == 0 {
		return "", fmt.Errorf("未启用 FRP 服务端")
	}
	var out strings.Builder
	for _, server := range servers {
		proxies, proxyErr := s.store.ListEnabledProxies(ctx, server.ID)
		if proxyErr != nil {
			return "", proxyErr
		}
		fmt.Fprintf(&out, "# 服务端：%s (%s:%d)\n", server.Name, server.ServerAddr, server.ServerPort)
		out.WriteString(renderMaskedTOML(server, proxies))
		out.WriteString("\n")
	}
	return out.String(), nil
}

func (s *Service) validateCredentials(in ServerInput, existing Server) error {
	if !in.Enabled {
		return nil
	}
	switch in.Options.AuthMethod {
	case "none":
		return nil
	case "token":
		if in.ClearToken || ((in.Token == "" || in.Token == maskedToken) && !existing.HasToken) {
			return fmt.Errorf("Token 认证需要填写 FRP Token")
		}
	case "oidc":
		if in.ClearOIDCClientSecret || ((in.OIDCClientSecret == "" || in.OIDCClientSecret == maskedToken) && !existing.HasOIDCSecret) {
			return fmt.Errorf("OIDC 认证需要填写客户端密钥")
		}
	}
	return nil
}

func (s *Service) encryptOptional(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return s.secretBox.Encrypt(value)
}

func (s *Service) saveOIDCSecret(ctx context.Context, serverID int64, in ServerInput) error {
	if in.ClearOIDCClientSecret {
		return s.store.SetOIDCSecret(ctx, serverID, "")
	}
	if in.OIDCClientSecret == "" || in.OIDCClientSecret == maskedToken {
		return nil
	}
	value, err := s.secretBox.Encrypt(in.OIDCClientSecret)
	if err != nil {
		return err
	}
	return s.store.SetOIDCSecret(ctx, serverID, value)
}

// saveDashboardPassword 保存 frps 管理接口密码（与 frpc Token 无关，仅用于 Havline 拉取流量信息）。
func (s *Service) saveDashboardPassword(ctx context.Context, serverID int64, in ServerInput) error {
	if in.ClearDashboardPassword {
		return s.store.SetDashboardPassword(ctx, serverID, "")
	}
	if in.DashboardPassword == "" || in.DashboardPassword == maskedToken {
		return nil
	}
	encoded, err := s.secretBox.Encrypt(in.DashboardPassword)
	if err != nil {
		return err
	}
	return s.store.SetDashboardPassword(ctx, serverID, encoded)
}

func (s *Service) loadAuthSecrets(ctx context.Context, server Server) (string, string, error) {
	if server.Options.AuthMethod == "none" {
		return "", "", nil
	}
	if server.Options.AuthMethod == "token" {
		encoded, err := s.store.Token(ctx, server.ID)
		if err != nil {
			return "", "", err
		}
		value, err := s.secretBox.Decrypt(encoded)
		if err != nil {
			return "", "", fmt.Errorf("FRP Token 无法解密，请重新保存服务端配置")
		}
		return value, "", nil
	}
	encoded, err := s.store.OIDCSecret(ctx, server.ID)
	if err != nil {
		return "", "", err
	}
	value, err := s.secretBox.Decrypt(encoded)
	if err != nil {
		return "", "", fmt.Errorf("OIDC 客户端密钥无法解密，请重新保存服务端配置")
	}
	return "", value, nil
}

func (s *Service) serverConfigured(server Server) bool {
	switch server.Options.AuthMethod {
	case "none":
		return true
	case "oidc":
		return server.HasOIDCSecret
	default:
		return server.HasToken
	}
}

func (s *Service) enrichTLSStatus(server *Server) {
	server.Options.TLSCertificateConfigured = s.existingTLSAssetPath(server.ID, "cert.pem") != ""
	server.Options.TLSKeyConfigured = s.existingTLSAssetPath(server.ID, "key.pem") != ""
	server.Options.TLSTrustedCAConfigured = s.existingTLSAssetPath(server.ID, "ca.pem") != ""
}

func (s *Service) tlsAssetPath(serverID int64, name string) string {
	return filepath.Join(s.cfg.FrpDir(), "certs", fmt.Sprintf("server-%d-%s", serverID, name))
}

func (s *Service) existingTLSAssetPath(serverID int64, name string) string {
	path := s.tlsAssetPath(serverID, name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func (s *Service) saveTLSAssets(serverID int64, in ServerInput) error {
	assets := []struct {
		name, content string
		clear         bool
	}{
		{"cert.pem", in.TLSCertificate, in.ClearTLSCertificate}, {"key.pem", in.TLSKey, in.ClearTLSKey}, {"ca.pem", in.TLSTrustedCA, in.ClearTLSTrustedCA},
	}
	for _, asset := range assets {
		path := s.tlsAssetPath(serverID, asset.name)
		if asset.clear {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if asset.content == "" {
			continue
		}
		if !strings.Contains(asset.content, "-----BEGIN") {
			return fmt.Errorf("TLS %s 内容不是有效 PEM", asset.name)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(strings.TrimSpace(asset.content)+"\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) persistConfig(ctx context.Context) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	return s.persistConfigsLocked(ctx)
}

// persistConfigsLocked 写全部服务端配置文件。调用方必须已持有 s.applyMu。
// 单个服务端写入失败立即返回，其余服务端保持上一版配置，不会出现部分新部分旧混写。
func (s *Service) persistConfigsLocked(ctx context.Context) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		path := s.serverConfigPath(server.ID)
		if !server.Enabled {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
			continue
		}
		if err := s.persistServerConfig(ctx, server); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) persistServerConfig(ctx context.Context, server Server) error {
	content, err := s.renderServerConfig(ctx, server)
	if err != nil {
		return err
	}
	return writeConfigAtomically(s.serverConfigPath(server.ID), content)
}

func writeConfigAtomically(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// 23.5：写入前把当前版本留存为 .bak。临时文件写入本身已原子（CreateTemp+Rename）,
	// 但 frpc verify 失败时上层需要回退目标,这里提供版本化备份。
	if err := fsutil.Backup(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "frpc-*.toml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
