package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fonu/fonu/internal/updater"
)

func main() {
	server, err := updater.NewServer(updater.Config{
		SocketPath:  envOr("FONU_UPDATER_SOCKET", "/run/fonu-updater/updater.sock"),
		ProjectDir:  envOr("FONU_UPDATE_PROJECT_DIR", "/workspace"),
		ComposeFile: envOr("FONU_UPDATE_COMPOSE_FILE", "docker-compose.yml"),
		Service:     envOr("FONU_UPDATE_SERVICE", "fonu"),
		Mode:        envOr("FONU_UPDATE_MODE", "local"),
		RepoURL:     envOr("FONU_UPDATE_REPO", ""),
		Branch:      envOr("FONU_UPDATE_BRANCH", "main"),
		AppURL:      envOr("FONU_UPDATE_APP_URL", "http://fonu:6893"),
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
