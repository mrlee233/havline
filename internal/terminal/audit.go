package terminal

import (
	"context"
	"database/sql"
	"time"
)

// AuditRetention 是审计记录的保留期：超过就由定时任务清理，避免表无限增长。
const AuditRetention = 90 * 24 * time.Hour

// AuditEntry 是一条终端会话审计记录，回答「谁、何时、连了哪台、多久、为什么断开」。
type AuditEntry struct {
	ID         int64  `json:"id,omitempty"`
	SessionID  string `json:"session_id"`
	ServerID   int64  `json:"server_id"`
	Target     string `json:"target"`
	User       string `json:"user"`
	ClientIP   string `json:"client_ip"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	DurationMS int64  `json:"duration_ms"`
	Reason     string `json:"reason,omitempty"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
}

// AuditSink 由会话管理器在会话结束时调用；未注入时审计只落日志。
type AuditSink interface {
	WriteAudit(ctx context.Context, entry AuditEntry) error
}

// AuditStore 把审计记录落到 SQLite，并提供查询与清理。
type AuditStore struct {
	db *sql.DB
}

func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// WriteAudit 写入一条会话审计。
func (s *AuditStore) WriteAudit(ctx context.Context, entry AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO terminal_audit_logs(
			session_id, server_id, target, username, client_ip,
			started_at, ended_at, duration_ms, reason, bytes_in, bytes_out
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.SessionID, entry.ServerID, entry.Target, entry.User, entry.ClientIP,
		entry.StartedAt, entry.EndedAt, entry.DurationMS, entry.Reason, entry.BytesIn, entry.BytesOut)
	return err
}

// ListAudits 按结束时间倒序返回最近的审计记录；limit 缺省 100，上限 500。
func (s *AuditStore) ListAudits(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, server_id, target, username, client_ip,
		       started_at, ended_at, duration_ms, COALESCE(reason, ''), bytes_in, bytes_out
		FROM terminal_audit_logs
		ORDER BY id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]AuditEntry, 0, limit)
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(
			&entry.ID, &entry.SessionID, &entry.ServerID, &entry.Target,
			&entry.User, &entry.ClientIP, &entry.StartedAt, &entry.EndedAt,
			&entry.DurationMS, &entry.Reason, &entry.BytesIn, &entry.BytesOut,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// PruneAudits 清理超过保留期的审计记录，返回删除条数。
func (s *AuditStore) PruneAudits(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-AuditRetention).Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `DELETE FROM terminal_audit_logs WHERE ended_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
