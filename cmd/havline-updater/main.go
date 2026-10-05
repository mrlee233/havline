package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/havline/havline/internal/updater"
)

func main() {
	token, tokenSource, err := updater.WaitForToken(
		envOr("HAVLINE_UPDATER_TOKEN", ""),
		envOr("HAVLINE_UPDATER_TOKEN_FILE", updater.DefaultTokenPath),
		2*time.Minute,
	)
	if err != nil {
		log.Fatal(err)
	}
	if token == "" {
		log.Fatal("HAVLINE_UPDATER_TOKEN 或 HAVLINE_UPDATER_TOKEN_FILE 未配置")
	}
	log.Printf("updater Token 已就绪：%s", tokenSource)

	server, err := updater.NewServer(updater.Config{
		SocketPath:     envOr("HAVLINE_UPDATER_SOCKET", "/run/havline-updater/updater.sock"),
		ProjectDir:     envOr("HAVLINE_UPDATE_PROJECT_DIR", "/workspace"),
		ComposeFile:    envOr("HAVLINE_UPDATE_COMPOSE_FILE", "docker-compose.yml"),
		Service:        envOr("HAVLINE_UPDATE_SERVICE", "havline"),
		Mode:           envOr("HAVLINE_UPDATE_MODE", "local"),
		RepoURL:        envOr("HAVLINE_UPDATE_REPO", ""),
		Branch:         envOr("HAVLINE_UPDATE_BRANCH", "main"),
		AppURL:         envOr("HAVLINE_UPDATE_APP_URL", "http://havline:6893"),
		HostProjectDir: envOr("HAVLINE_UPDATE_HOST_DIR", ""),
		ProjectName:    envOr("HAVLINE_UPDATE_PROJECT_NAME", ""),
		SelfImage:      envOr("HAVLINE_UPDATE_SELF_IMAGE", ""),
		DockerSocket:   envOr("HAVLINE_UPDATE_DOCKER_SOCK", "/var/run/docker.sock"),
		Token:          token,
	}, nil)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.Serve(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
