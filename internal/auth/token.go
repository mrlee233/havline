package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// API Token 给无法保存会话 cookie 的调用方（脚本、HomeAssistant、手机快捷指令）用。
//
// 明文只在创建响应里返回一次，库里只存 SHA-256：令牌由 crypto/rand 生成、熵足够高，
// 不像人类密码那样需要 bcrypt 这类慢哈希（慢哈希在这里只会拖慢每个 API 请求）。
const (
	TokenScopeRead  = "read"
	TokenScopeWrite = "write"

	// lastUsedWriteInterval 限制回写频率：单连接 SQLite 上每个请求都写一次不划算，
	// 同一分钟内重复使用不再写。
	lastUsedWriteInterval = time.Minute

	maxTokenNameLength = 64
)

type APIToken struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// CreatedAPIToken 只在创建响应里出现：Token 是明文，之后无法再取回。
type CreatedAPIToken struct {
	APIToken
	Token string `json:"token"`
}

func (s *Service) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, scope, COALESCE(expires_at, ''), COALESCE(last_used_at, ''), created_at
		FROM api_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]APIToken, 0, 8)
	for rows.Next() {
		var token APIToken
		if err := rows.Scan(&token.ID, &token.Name, &token.Scope, &token.ExpiresAt, &token.LastUsedAt, &token.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

// CreateAPIToken 新建令牌并返回一次明文；expiresAt 为零值表示永不过期。
func (s *Service) CreateAPIToken(ctx context.Context, name, scope string, expiresAt time.Time) (CreatedAPIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CreatedAPIToken{}, fmt.Errorf("请填写令牌名称")
	}
	if len([]rune(name)) > maxTokenNameLength {
		return CreatedAPIToken{}, fmt.Errorf("令牌名称最长 %d 个字", maxTokenNameLength)
	}
	if scope != TokenScopeRead && scope != TokenScopeWrite {
		return CreatedAPIToken{}, fmt.Errorf("无效的权限范围")
	}

	plain, err := randomToken(32)
	if err != nil {
		return CreatedAPIToken{}, err
	}
	expires := ""
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC().Format(time.RFC3339)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens(name, token_hash, scope, expires_at) VALUES (?, ?, ?, ?)`,
		name, hashToken(plain), scope, expires)
	if err != nil {
		return CreatedAPIToken{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CreatedAPIToken{}, err
	}
	token, err := s.apiTokenByID(ctx, id)
	if err != nil {
		return CreatedAPIToken{}, err
	}
	return CreatedAPIToken{APIToken: token, Token: plain}, nil
}

func (s *Service) DeleteAPIToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("令牌不存在")
	}
	return nil
}

// ValidateAPIToken 校验明文令牌并返回权限范围；校验通过后限频回写最后使用时间。
func (s *Service) ValidateAPIToken(ctx context.Context, plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", ErrUnauthorized
	}

	var (
		id        int64
		scope     string
		expiresAt string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, scope, COALESCE(expires_at, '') FROM api_tokens WHERE token_hash = ?`, hashToken(plain)).
		Scan(&id, &scope, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUnauthorized
		}
		return "", err
	}
	if expiresAt != "" {
		expires, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil || time.Now().After(expires) {
			return "", ErrUnauthorized
		}
	}

	now := time.Now().UTC()
	cutoff := now.Add(-lastUsedWriteInterval).Format(time.RFC3339)
	// 回写失败不影响本次调用：last_used_at 只是审计信息
	_, _ = s.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at < ?)`,
		now.Format(time.RFC3339), id, cutoff)
	return scope, nil
}

func (s *Service) apiTokenByID(ctx context.Context, id int64) (APIToken, error) {
	var token APIToken
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, scope, COALESCE(expires_at, ''), COALESCE(last_used_at, ''), created_at
		FROM api_tokens WHERE id = ?`, id).
		Scan(&token.ID, &token.Name, &token.Scope, &token.ExpiresAt, &token.LastUsedAt, &token.CreatedAt)
	return token, err
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
