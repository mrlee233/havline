package frp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type renderedConfig struct {
	Server           Server
	Token            string
	OIDCClientSecret string
	LogPath          string
	TLSCertPath      string
	TLSKeyPath       string
	TLSTrustedCAPath string
	Proxies          []Proxy
}

func renderTOML(cfg renderedConfig) string {
	var out strings.Builder
	writeKeyValue(&out, "serverAddr", cfg.Server.ServerAddr)
	writeKeyValue(&out, "serverPort", cfg.Server.ServerPort)
	options := normalizeServerOptions(cfg.Server.Options)
	if options.User != "" {
		writeKeyValue(&out, "user", options.User)
	}
	if options.AuthMethod != "none" {
		writeKeyValue(&out, "auth.method", options.AuthMethod)
	}
	if options.AuthMethod == "token" {
		writeKeyValue(&out, "auth.token", cfg.Token)
	}
	if options.AuthMethod == "oidc" {
		writeKeyValue(&out, "auth.oidc.clientID", options.OIDCClientID)
		writeKeyValue(&out, "auth.oidc.clientSecret", cfg.OIDCClientSecret)
		writeKeyValue(&out, "auth.oidc.tokenEndpointURL", options.OIDCTokenEndpointURL)
		if options.OIDCAudience != "" {
			writeKeyValue(&out, "auth.oidc.audience", options.OIDCAudience)
		}
		if options.OIDCScope != "" {
			writeKeyValue(&out, "auth.oidc.scope", options.OIDCScope)
		}
	}
	if cfg.LogPath != "" {
		writeKeyValue(&out, "log.to", cfg.LogPath)
	}
	writeKeyValue(&out, "log.level", options.LogLevel)
	writeKeyValue(&out, "log.maxDays", options.LogMaxDays)
	writeKeyValue(&out, "transport.protocol", options.Protocol)
	writeKeyValue(&out, "transport.tls.enable", cfg.Server.TLS)
	writeKeyValue(&out, "transport.tls.disableCustomTLSFirstByte", options.TLSDisableCustomFirstByte)
	if cfg.Server.TLSServerName != "" {
		writeKeyValue(&out, "transport.tls.serverName", cfg.Server.TLSServerName)
	}
	if cfg.TLSCertPath != "" {
		writeKeyValue(&out, "transport.tls.certFile", cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != "" {
		writeKeyValue(&out, "transport.tls.keyFile", cfg.TLSKeyPath)
	}
	if cfg.TLSTrustedCAPath != "" {
		writeKeyValue(&out, "transport.tls.trustedCaFile", cfg.TLSTrustedCAPath)
	}
	if options.ProxyURL != "" {
		writeKeyValue(&out, "transport.proxyURL", options.ProxyURL)
	}
	writeKeyValue(&out, "transport.dialServerTimeout", options.DialServerTimeout)
	writeKeyValue(&out, "transport.dialServerKeepalive", options.DialServerKeepalive)
	if options.ConnectServerLocalIP != "" {
		writeKeyValue(&out, "transport.connectServerLocalIP", options.ConnectServerLocalIP)
	}
	if options.DNSServer != "" {
		writeKeyValue(&out, "transport.dnsServer", options.DNSServer)
	}
	writeKeyValue(&out, "transport.poolCount", options.PoolCount)
	writeKeyValue(&out, "transport.tcpMux", options.TCPMux)
	writeKeyValue(&out, "transport.tcpMuxKeepaliveInterval", options.TCPMuxKeepaliveInterval)
	writeKeyValue(&out, "transport.heartbeatInterval", options.HeartbeatInterval)
	writeKeyValue(&out, "transport.heartbeatTimeout", options.HeartbeatTimeout)
	writeKeyValue(&out, "loginFailExit", options.LoginFailExit)

	for _, proxy := range cfg.Proxies {
		out.WriteString("\n[[proxies]]\n")
		writeKeyValue(&out, "name", proxy.Name)
		writeKeyValue(&out, "type", proxy.Type)
		writeKeyValue(&out, "localIP", proxy.LocalIP)
		writeKeyValue(&out, "localPort", proxy.LocalPort)
		if proxy.Type == "tcp" && proxy.RemotePort != nil {
			writeKeyValue(&out, "remotePort", *proxy.RemotePort)
		}
		if proxy.Type == "udp" && proxy.RemotePort != nil {
			writeKeyValue(&out, "remotePort", *proxy.RemotePort)
		}
		if proxy.Type == "http" || proxy.Type == "https" || proxy.Type == "tcpmux" {
			writeStringSlice(&out, "customDomains", proxy.CustomDomains)
		}
		if proxy.HostHeaderRewrite != "" {
			writeKeyValue(&out, "hostHeaderRewrite", proxy.HostHeaderRewrite)
		}
		writeProxyOptions(&out, proxy)
	}
	return out.String()
}

func writeProxyOptions(out *strings.Builder, proxy Proxy) {
	opts := proxy.Options
	if opts.Subdomain != "" {
		writeKeyValue(out, "subdomain", opts.Subdomain)
	}
	if len(opts.Locations) > 0 {
		writeStringSlice(out, "locations", opts.Locations)
	}
	if opts.RouteByHTTPUser != "" {
		writeKeyValue(out, "routeByHTTPUser", opts.RouteByHTTPUser)
	}
	if opts.HTTPUser != "" {
		writeKeyValue(out, "httpUser", opts.HTTPUser)
	}
	if opts.HTTPPassword != "" {
		writeKeyValue(out, "httpPassword", opts.HTTPPassword)
	}
	if opts.Multiplexer != "" {
		writeKeyValue(out, "multiplexer", opts.Multiplexer)
	}
	if opts.SecretKey != "" {
		writeKeyValue(out, "secretKey", opts.SecretKey)
	}
	if len(opts.AllowUsers) > 0 {
		writeStringSlice(out, "allowUsers", opts.AllowUsers)
	}
	writeProxyTransport(out, opts.Transport)
	writeProxyHealthCheck(out, opts.HealthCheck)
	if opts.LoadBalancer.Group != "" {
		writeKeyValue(out, "loadBalancer.group", opts.LoadBalancer.Group)
	}
	if opts.LoadBalancer.GroupKey != "" {
		writeKeyValue(out, "loadBalancer.groupKey", opts.LoadBalancer.GroupKey)
	}
	writeStringMap(out, "metadatas", opts.Metadatas)
	writeStringMap(out, "requestHeaders.set", opts.RequestHeaders)
	writeStringMap(out, "responseHeaders.set", opts.ResponseHeaders)
	// natTraversal 是较新版本 frpc 才支持的字段，旧版 frpc 会把未知字段判为校验错误；
	// disableAssistedAddrs=false 与 frp 默认行为等价，因此仅在显式禁用时写入。
	if opts.NATTraversal != nil && opts.NATTraversal.DisableAssistedAddrs {
		writeKeyValue(out, "natTraversal.disableAssistedAddrs", true)
	}
	if opts.Plugin != nil {
		writeProxyPlugin(out, *opts.Plugin)
	}
}

func writeProxyTransport(out *strings.Builder, opts ProxyTransportOptions) {
	if opts.BandwidthLimit != "" {
		writeKeyValue(out, "transport.bandwidthLimit", opts.BandwidthLimit)
	}
	if opts.BandwidthLimitMode != "" {
		writeKeyValue(out, "transport.bandwidthLimitMode", opts.BandwidthLimitMode)
	}
	if opts.UseEncryption {
		writeKeyValue(out, "transport.useEncryption", true)
	}
	if opts.UseCompression {
		writeKeyValue(out, "transport.useCompression", true)
	}
	if opts.ProxyProtocolVersion != "" {
		writeKeyValue(out, "transport.proxyProtocolVersion", opts.ProxyProtocolVersion)
	}
}

func writeProxyHealthCheck(out *strings.Builder, opts ProxyHealthCheckOptions) {
	if opts.Type == "" {
		return
	}
	writeKeyValue(out, "healthCheck.type", opts.Type)
	if opts.Path != "" {
		writeKeyValue(out, "healthCheck.path", opts.Path)
	}
	if opts.IntervalSeconds > 0 {
		writeKeyValue(out, "healthCheck.intervalSeconds", opts.IntervalSeconds)
	}
	if opts.MaxFailed > 0 {
		writeKeyValue(out, "healthCheck.maxFailed", opts.MaxFailed)
	}
	if opts.TimeoutSeconds > 0 {
		writeKeyValue(out, "healthCheck.timeoutSeconds", opts.TimeoutSeconds)
	}
	if len(opts.HTTPHeaders) > 0 {
		values := make([]string, 0, len(opts.HTTPHeaders))
		for _, header := range opts.HTTPHeaders {
			values = append(values, fmt.Sprintf("{ name = %s, value = %s }", strconv.Quote(header.Name), strconv.Quote(header.Value)))
		}
		fmt.Fprintf(out, "healthCheck.httpHeaders = [%s]\n", strings.Join(values, ", "))
	}
}

func writeProxyPlugin(out *strings.Builder, plugin ProxyPluginOptions) {
	out.WriteString("[proxies.plugin]\n")
	writeKeyValue(out, "type", plugin.Type)
	for _, item := range []struct{ key, value string }{
		{"unixPath", plugin.UnixPath}, {"localPath", plugin.LocalPath}, {"stripPrefix", plugin.StripPrefix},
		{"localAddr", plugin.LocalAddr}, {"httpUser", plugin.HTTPUser}, {"httpPassword", plugin.HTTPPassword},
		{"username", plugin.Username}, {"password", plugin.Password}, {"crtPath", plugin.CRTPath}, {"keyPath", plugin.KeyPath},
		{"hostHeaderRewrite", plugin.HostHeaderRewrite}, {"destinationIP", plugin.DestinationIP},
	} {
		if item.value != "" {
			writeKeyValue(out, item.key, item.value)
		}
	}
}

func writeStringMap(out *strings.Builder, prefix string, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key != "" {
			writeKeyValue(out, prefix+"."+key, values[key])
		}
	}
}

func renderMaskedTOML(server Server, proxies []Proxy) string {
	return renderTOML(renderedConfig{Server: server, Token: "********", OIDCClientSecret: "********", Proxies: proxies})
}

func writeKeyValue(out *strings.Builder, key string, value any) {
	switch v := value.(type) {
	case string:
		fmt.Fprintf(out, "%s = %s\n", key, strconv.Quote(v))
	case int:
		fmt.Fprintf(out, "%s = %d\n", key, v)
	case bool:
		fmt.Fprintf(out, "%s = %t\n", key, v)
	}
}

func writeStringSlice(out *strings.Builder, key string, values []string) {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	fmt.Fprintf(out, "%s = [%s]\n", key, strings.Join(quoted, ", "))
}
