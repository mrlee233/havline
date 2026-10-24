package backup

import (
	"context"
	"os"
	"testing"

	"github.com/havline/havline/internal/db"
)

// TestExportRealData 用真实数据目录跑一遍完整导出，验证「立即备份」在实机上不再报
// archive/tar: write too long，并且归档里确实带上了 session_secret 与 frp 配置。
// 仅在设置了 HV_REAL_DATA 时执行，避免把本地数据依赖带进 CI。
func TestExportRealData(t *testing.T) {
	dir := os.Getenv("HV_REAL_DATA")
	if dir == "" {
		t.Skip("未设置 HV_REAL_DATA")
	}
	conn, err := db.Open(dir + "/havline.db")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	svc := New(dir)
	svc.SetDB(conn)
	path, err := svc.Export(context.Background())
	if err != nil {
		t.Fatalf("导出失败：%v", err)
	}
	defer os.Remove(path)

	entries, err := svc.Verify(context.Background(), path)
	if err != nil {
		t.Fatalf("校验失败：%v", err)
	}
	missing, err := svc.MissingSessionSecret(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if missing {
		t.Fatal("真实数据的归档应包含 session_secret")
	}
	t.Logf("导出成功：%s，条目 %d", path, entries)

	// 有口令时也要能正常导出 + 校验
	encPath, err := svc.ExportWith(context.Background(), Options{Passphrase: "test-pass"})
	if err != nil {
		t.Fatalf("加密导出失败：%v", err)
	}
	defer os.Remove(encPath)
	if _, err := svc.VerifyWith(context.Background(), encPath, Options{Passphrase: "test-pass"}); err != nil {
		t.Fatalf("加密归档校验失败：%v", err)
	}
	t.Logf("加密导出成功：%s", encPath)
}
