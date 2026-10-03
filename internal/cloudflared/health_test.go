package cloudflared

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/config"
	"github.com/havline/havline/internal/db"
)

func TestComputeHealthUptimeExcludesUnknown(t *testing.T) {
	from := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	events := []HealthEvent{
		{TunnelID: 1, Domain: "a.example.com", State: HealthUnknown, At: from.Format(time.RFC3339)},
		{TunnelID: 1, Domain: "a.example.com", State: HealthUp, At: from.Add(30 * time.Minute).Format(time.RFC3339)},
		{TunnelID: 1, Domain: "a.example.com", State: HealthDown, At: from.Add(90 * time.Minute).Format(time.RFC3339)},
	}
	stats := ComputeHealthUptime(events, from, to)
	if len(stats) != 1 {
		t.Fatalf("应返回一条统计：%#v", stats)
	}
	if got := stats[0].Uptime; got < 66.6 || got > 66.7 || stats[0].UnknownMinutes != 30 || stats[0].DownMinutes != 30 {
		t.Fatalf("可用率计算不符：%#v", stats[0])
	}
}

func TestRecordHealthSkipsUnchangedState(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	svc := NewService(config.Config{DataDir: dir}, NewStore(conn), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	sample := []HealthSample{{TunnelID: 1, Domain: "a.example.com", State: HealthUp}}
	if err := svc.RecordHealth(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordHealth(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM cf_health_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("同状态不应重复写入，实际 %d 条", count)
	}
}
