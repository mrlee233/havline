package terminal

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/havline/havline/internal/db"
)

// TestCommandStoreRoundTrip：保存、列表、删除快捷命令，并校验输入约束。
func TestCommandStoreRoundTrip(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "commands.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	defer conn.Close()
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}

	store := NewCommandStore(conn)
	ctx := context.Background()

	first, err := store.CreateCommand(ctx, "  磁盘占用  ", "df -h\n")
	if err != nil {
		t.Fatalf("保存命令失败：%v", err)
	}
	if first.Name != "磁盘占用" {
		t.Fatalf("名称应当去掉首尾空格：%q", first.Name)
	}
	if first.Command != "df -h" {
		t.Fatalf("命令应当去掉尾部换行：%q", first.Command)
	}
	if first.ID == 0 || first.CreatedAt == "" {
		t.Fatalf("保存结果缺少 ID 或时间：%+v", first)
	}
	if _, err := store.CreateCommand(ctx, "systemctl", "systemctl status nginx"); err != nil {
		t.Fatalf("保存第二条失败：%v", err)
	}

	list, err := store.ListCommands(ctx)
	if err != nil {
		t.Fatalf("列表失败：%v", err)
	}
	if len(list) != 2 {
		t.Fatalf("期望 2 条，实际 %d 条", len(list))
	}
	// 倒序：后加的在前
	if list[0].Name != "systemctl" {
		t.Fatalf("排序不符：%+v", list)
	}

	if err := store.DeleteCommand(ctx, first.ID); err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if err := store.DeleteCommand(ctx, first.ID); !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("重复删除应当 ErrCommandNotFound，实际 %v", err)
	}
	rest, err := store.ListCommands(ctx)
	if err != nil || len(rest) != 1 {
		t.Fatalf("删除后应当剩 1 条：%v %d", err, len(rest))
	}

	// 输入约束
	if _, err := store.CreateCommand(ctx, "  ", "ls"); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("空名称应当报错，实际 %v", err)
	}
	if _, err := store.CreateCommand(ctx, "空白命令", "   "); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("空命令应当报错，实际 %v", err)
	}
	if _, err := store.CreateCommand(ctx, strings.Repeat("长", maxCommandNameLength+1), "ls"); err == nil {
		t.Fatal("超长名称应当报错")
	}
}
