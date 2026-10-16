package terminal

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	// maxCommandNameLength 是快捷命令名称的长度上限（按字符算）
	maxCommandNameLength = 64
	// maxCommandLength 是命令正文的字节上限：够写复杂管道，但塞不进整个脚本文件
	maxCommandLength = 4096
)

var (
	// ErrInvalidCommand 表示名称或命令内容不合法。
	ErrInvalidCommand = errors.New("命令名称与内容都不能为空")
	// ErrCommandNotFound 表示快捷命令不存在。
	ErrCommandNotFound = errors.New("快捷命令不存在")
)

// TerminalCommand 是保存的一条快捷命令。
type TerminalCommand struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
}

// CommandStore 把快捷命令落 SQLite：换浏览器也在，与终端会话无关。
type CommandStore struct {
	db *sql.DB
}

func NewCommandStore(db *sql.DB) *CommandStore {
	return &CommandStore{db: db}
}

// ListCommands 按新增时间倒序返回全部快捷命令。
func (s *CommandStore) ListCommands(ctx context.Context) ([]TerminalCommand, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, command, created_at FROM terminal_commands ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TerminalCommand, 0, 16)
	for rows.Next() {
		var item TerminalCommand
		if err := rows.Scan(&item.ID, &item.Name, &item.Command, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// CreateCommand 保存一条快捷命令：名称与内容必填，尾部的换行会被去掉（发送时再补回车）。
func (s *CommandStore) CreateCommand(ctx context.Context, name, command string) (TerminalCommand, error) {
	trimmedName := strings.TrimSpace(name)
	trimmedCommand := strings.TrimRight(strings.ReplaceAll(command, "\r\n", "\n"), "\n")
	if trimmedName == "" || strings.TrimSpace(trimmedCommand) == "" {
		return TerminalCommand{}, ErrInvalidCommand
	}
	if len([]rune(trimmedName)) > maxCommandNameLength {
		return TerminalCommand{}, errors.New("命令名称过长")
	}
	if len(trimmedCommand) > maxCommandLength {
		return TerminalCommand{}, errors.New("命令内容过长")
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO terminal_commands(name, command, created_at) VALUES (?, ?, ?)`,
		trimmedName, trimmedCommand, createdAt)
	if err != nil {
		return TerminalCommand{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return TerminalCommand{}, err
	}
	return TerminalCommand{ID: id, Name: trimmedName, Command: trimmedCommand, CreatedAt: createdAt}, nil
}

// DeleteCommand 删除一条快捷命令；不存在时返回 ErrCommandNotFound。
func (s *CommandStore) DeleteCommand(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM terminal_commands WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrCommandNotFound
	}
	return nil
}
