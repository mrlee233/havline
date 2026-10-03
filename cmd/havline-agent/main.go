package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/havline/havline/internal/agent"
)

func main() {
	listen := flag.String("listen", envOr("HAVLINE_AGENT_LISTEN", "127.0.0.1:7700"), "agent listen address")
	allowRemote := flag.Bool("allow-remote", envBool("HAVLINE_AGENT_ALLOW_REMOTE"), "allow non-loopback listen address")
	token := flag.String("token", os.Getenv("HAVLINE_AGENT_TOKEN"), "agent bearer token")
	nginxDir := flag.String("nginx-conf-dir", envOr("HAVLINE_AGENT_NGINX_CONF_DIR", ""), "managed nginx config directory（留空自动探测 Nginx 实际加载的 vhost 目录）")
	certsDir := flag.String("certs-dir", envOr("HAVLINE_AGENT_CERTS_DIR", "/var/lib/havline-agent/certs"), "certificate directory")
	frpsConfig := flag.String("frps-config", envOr("HAVLINE_AGENT_FRPS_CONFIG", "/etc/frp/frps.toml"), "frps config path")
	frpsBin := flag.String("frps-bin", envOr("HAVLINE_AGENT_FRPS_BIN", "frps"), "frps binary")
	dataDir := flag.String("data-dir", envOr("HAVLINE_AGENT_DATA_DIR", "/var/lib/havline-agent"), "agent data directory")
	showVersion := flag.Bool("version", false, "print agent version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("havline-agent", agent.AgentVersion)
		return
	}
	if *token == "" {
		log.Fatal("HAVLINE_AGENT_TOKEN or -token is required")
	}
	server, err := agent.NewServer(agent.Config{
		ListenAddr: *listen, AllowRemote: *allowRemote, Token: *token, NginxConfDir: *nginxDir,
		CertsDir: *certsDir, FrpsConfigPath: *frpsConfig, FrpsBinary: *frpsBin, DataDir: *dataDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
