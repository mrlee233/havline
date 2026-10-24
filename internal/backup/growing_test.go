package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/havline/havline/internal/db"
)

// openTestDB 建一个跑过迁移的临时库，用于验证快照路径。
func openTestDB(t *testing.T) (*sql.DB, error) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(context.Background(), conn, filepath.Join("..", "..", "migrations")); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// TestExportSurvivesGrowingFiles 复现「立即备份」报 archive/tar: write too long 的现场。
//
// 成因：addFile 用 WalkDir 那一刻拿到的 FileInfo.Size() 写 tar 头，随后才去读文件内容。
// 而 logs、frpc 配置这些文件在打包期间还在长，读到的字节多于头部声明，tar 直接报错、
// 整份备份作废。这里的做法是让一个「边打包边追加」的写入者持续往文件里写。
func TestExportSurvivesGrowingFiles(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "havline.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 造一个会持续增长的文件，放在会被归档的目录下
	frpDir := filepath.Join(dataDir, "frp")
	if err := os.MkdirAll(frpDir, 0o750); err != nil {
		t.Fatal(err)
	}
	growing := filepath.Join(frpDir, "frpc-server-1.toml")
	if err := os.WriteFile(growing, []byte("token = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		chunk := []byte("padding = \"" + "0123456789abcdef" + "\"\n")
		for {
			select {
			case <-stop:
				return
			default:
			}
			f, err := os.OpenFile(growing, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return
			}
			_, _ = f.Write(chunk)
			_ = f.Close()
			time.Sleep(time.Millisecond)
		}
	}()

	path, err := New(dataDir).Export(context.Background())
	close(stop)
	<-done

	if err != nil {
		t.Fatalf("文件在打包期间增长时导出不应失败：%v", err)
	}
	if _, err := New(dataDir).Verify(context.Background(), path); err != nil {
		t.Fatalf("导出的归档应可校验：%v", err)
	}
}

// TestExportUsesDatabaseSnapshot：注入数据库后，归档里的 havline.db 必须来自快照，
// 而不是直接读主文件——主文件在写入时并不自洽。
func TestExportUsesDatabaseSnapshot(t *testing.T) {
	dataDir := t.TempDir()
	// 故意在主文件里放一段「不是有效 SQLite」的内容：
	// 若实现退回直接打包文件，归档里就会是这串字节。
	if err := os.WriteFile(filepath.Join(dataDir, "havline.db"), []byte("not-a-valid-db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "session_secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	conn, err := openTestDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	svc := New(dataDir)
	svc.SetDB(conn)
	path, err := svc.Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	content := readArchiveEntry(t, path, "havline.db")
	if string(content) == "not-a-valid-db" {
		t.Fatal("归档里的数据库应来自 VACUUM INTO 快照，而不是直接读取主文件")
	}
	// SQLite 文件头是 "SQLite format 3\x00"
	if len(content) < 16 || string(content[:15]) != "SQLite format 3" {
		t.Fatalf("快照应是有效 SQLite 文件，实际头部：%q", content[:min(16, len(content))])
	}
}

func readArchiveEntry(t *testing.T, path, want string) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != want {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
}
