package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	dataDir string
	// db 用于把数据库导出成一致性快照。为 nil 时退回「直接打包数据库文件」，
	// 后者只在测试或不接数据库的场景下使用。
	db *sql.DB
}

// Options 是导出 / 恢复的可选项；Passphrase 非空时归档会被口令加密。
type Options struct {
	Passphrase string
}

// archiveEntries 是要打进归档的数据。
//
// 必须包含 session_secret：它同时是 secret.Box 的 AES-GCM 密钥，
// 少了它则数据库里的 DDNS / FRP / Cloudflare / 通知凭据在换机恢复后全部解不开，
// 界面表现为「配置都在，但一执行就报解密失败」。
// 必须包含 frp：frpc 的 toml 里有明文 token，缺了它穿透服务需要重新配一遍。
var archiveEntries = []string{"havline.db", "session_secret", "certs", "nginx", "frp", "cloudflared/tunnels"}

// 还原时的固定权限：备份内容只有数据库、证书与 nginx 配置，不接受归档自带的权限位
const (
	restoreFileMode = 0o640
	restoreDirMode  = 0o750
	// 密钥文件的权限单独收紧：它是解密库里全部凭据的那把钥匙
	restoreSecretMode = 0o600
)

func New(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

// SetDB 注入数据库连接，导出一致性快照时使用。
// 不放进 New 的签名里：现有调用点（含测试）很多只关心文件集合。
func (s *Service) SetDB(db *sql.DB) {
	s.db = db
}

// backupTimeFmt 归档文件名里的时间戳；List / Path 都按同一套格式校验
const backupTimeFmt = "20060102-150405"

// Export 导出一份未加密归档并返回路径；文件留在 <DataDir>/backups 下，
// 由调用方决定下载后删除还是长期保留。
func (s *Service) Export(ctx context.Context) (string, error) {
	return s.ExportWith(ctx, Options{})
}

// ExportWith 导出一份归档；Passphrase 非空时用口令加密。
func (s *Service) ExportWith(ctx context.Context, opts Options) (string, error) {
	if err := os.MkdirAll(s.backupsDir(), 0o755); err != nil {
		return "", err
	}

	// 先写到暂存文件：加密要先拿到完整的 tar.gz，且暂存名不匹配备份命名规则，
	// 中途失败不会在列表里留下一个看起来能用的残件。
	staging, err := os.CreateTemp(s.backupsDir(), "havline-backup-*.tar.gz.tmp")
	if err != nil {
		return "", err
	}
	stagingPath := staging.Name()
	defer os.Remove(stagingPath)
	if err := s.writeArchive(ctx, staging, stagingPath); err != nil {
		return "", err
	}

	// CreateTemp 保证同一秒内多次导出不会覆盖已有备份（文件名带随机后缀）
	final, err := os.CreateTemp(s.backupsDir(), fmt.Sprintf("havline-backup-%s-*.tar.gz", time.Now().Format(backupTimeFmt)))
	if err != nil {
		return "", err
	}
	finalPath := final.Name()
	if err := final.Close(); err != nil {
		_ = os.Remove(finalPath)
		return "", err
	}
	_ = os.Chmod(finalPath, 0o600)

	passphrase := strings.TrimSpace(opts.Passphrase)
	if passphrase == "" {
		// 未加密：直接把暂存文件改名成正式备份名
		if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Rename(stagingPath, finalPath); err != nil {
			return "", err
		}
		return finalPath, nil
	}

	src, err := os.Open(stagingPath)
	if err != nil {
		_ = os.Remove(finalPath)
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(finalPath, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = os.Remove(finalPath)
		return "", err
	}
	if err := sealFile(dst, src, passphrase); err != nil {
		_ = dst.Close()
		_ = os.Remove(finalPath)
		return "", err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(finalPath)
		return "", err
	}
	return finalPath, nil
}

// writeArchive 写归档，逐层检查关闭错误。
//
// Close 的错误必须往上抛：tar/gzip 的尾部与文件系统的 flush 都发生在 Close，
// 忽略它就会出现「备份文件不完整、函数却返回成功」——这是最坏的一种假成功。
// 任一步失败都删掉半成品，不留一个看起来能用的备份。
func (s *Service) writeArchive(ctx context.Context, file *os.File, outPath string) error {
	discard := func() {
		_ = file.Close()
		_ = os.Remove(outPath)
	}

	gw := gzip.NewWriter(file)
	tw := tar.NewWriter(gw)
	for _, entry := range archiveEntries {
		// havline.db 走一致性快照，其余（证书 / nginx / frp / 密钥）直接打包
		if entry == databaseName {
			if err := s.addDatabase(ctx, tw); err != nil {
				discard()
				return err
			}
			continue
		}
		if err := addToArchive(ctx, tw, s.dataDir, entry); err != nil {
			discard()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		discard()
		return err
	}
	if err := gw.Close(); err != nil {
		discard()
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(outPath)
		return err
	}
	return nil
}

// databaseName 是归档里的数据库条目名，也是恢复时的落点文件名。
const databaseName = "havline.db"

// addDatabase 把数据库打成一份一致性快照。
//
// 不能直接读 havline.db 文件：SQLite 在写入过程中，主文件与 -wal / -journal
// 的内容随时在变，直接 `cp` 出来的副本可能是「结构完整但内容自相矛盾」的库
// （恢复后表现为表锁死、索引与数据不一致）。VACUUM INTO 由 SQLite 自己保证
// 快照一致性，且不需要停机。
func (s *Service) addDatabase(ctx context.Context, tw *tar.Writer) error {
	if s.db == nil {
		// 没接数据库（测试或旧调用点）：退回直接打包文件
		return addToArchive(ctx, tw, s.dataDir, databaseName)
	}

	snapshot, err := os.CreateTemp("", "havline-snapshot-*.db")
	if err != nil {
		return err
	}
	snapshotPath := snapshot.Name()
	// VACUUM INTO 要求目标文件不存在；CreateTemp 已经把文件创建出来了
	if err := snapshot.Close(); err != nil {
		_ = os.Remove(snapshotPath)
		return err
	}
	if err := os.Remove(snapshotPath); err != nil {
		return err
	}
	defer os.Remove(snapshotPath)

	// 参数不能直接拼在 SQL 里（VACUUM INTO 支持字符串字面量，但不支持占位符），
	// 所以这里做转义：路径里的单引号在 Windows 临时目录下可能出现。
	quoted := strings.ReplaceAll(snapshotPath, "'", "''")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`); err != nil {
		return fmt.Errorf("导出数据库快照失败：%w", err)
	}

	info, err := os.Stat(snapshotPath)
	if err != nil {
		return err
	}
	file, err := os.Open(snapshotPath)
	if err != nil {
		return err
	}
	defer file.Close()

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = databaseName
	header.Size = info.Size()
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	written, err := io.Copy(tw, io.LimitReader(file, info.Size()))
	if err != nil {
		return err
	}
	if written < info.Size() {
		if _, err := io.CopyN(tw, zeroReader{}, info.Size()-written); err != nil {
			return err
		}
	}
	return nil
}

// Restore 还原一份未加密归档。
func (s *Service) Restore(ctx context.Context, archivePath string) error {
	return s.RestoreWith(ctx, archivePath, Options{})
}

// RestoreWith 还原归档；归档被口令加密时必须提供 Passphrase。
func (s *Service) RestoreWith(ctx context.Context, archivePath string, opts Options) error {
	encrypted, err := IsEncrypted(archivePath)
	if err != nil {
		return err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file
	if encrypted {
		passphrase := strings.TrimSpace(opts.Passphrase)
		if passphrase == "" {
			return ErrPassphraseRequired
		}
		plain, err := openSealedFile(file, passphrase)
		if err != nil {
			return err
		}
		reader = plain
	}

	gr, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("无效的备份文件")
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			continue
		}
		target := filepath.Join(s.dataDir, header.Name)
		// 归档条目必须落在 DataDir 内：前缀比较会被 ../data-other 这类同前缀路径绕过，用 Rel 判定
		rel, err := filepath.Rel(s.dataDir, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("备份条目非法: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, restoreDirMode); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), restoreDirMode); err != nil {
				return err
			}
			// 归档里的权限位不照搬（构造的归档可带 0777 / 执行位）：备份内容只有数据库、证书与 nginx 配置
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, restoreModeFor(header.Name))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// restoreModeFor 决定还原后的权限：session_secret 是解密凭据的钥匙，单独收紧到 0600。
func restoreModeFor(name string) os.FileMode {
	if filepath.Base(filepath.ToSlash(name)) == "session_secret" {
		return restoreSecretMode
	}
	return restoreFileMode
}

func addToArchive(ctx context.Context, tw *tar.Writer, baseDir, rel string) error {
	path := filepath.Join(baseDir, rel)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return addDir(ctx, tw, baseDir, rel)
	}
	return addFile(tw, baseDir, rel, info)
}

// skipArchivePath 排除不该进归档的内容：
//   - backups/：备份里套备份会指数级膨胀；
//   - frp/bin、frp/downloads：frpc 二进制与下载缓存，体积大且可重新下载；
//   - frp/*.log：运行日志没有恢复价值。
func skipArchivePath(rel string) bool {
	slash := filepath.ToSlash(rel)
	if strings.Contains("/"+slash+"/", "/backups/") {
		return true
	}
	if slash == "frp/bin" || strings.HasPrefix(slash, "frp/bin/") {
		return true
	}
	if slash == "frp/downloads" || strings.HasPrefix(slash, "frp/downloads/") {
		return true
	}
	return strings.HasPrefix(slash, "frp/") && strings.HasSuffix(slash, ".log")
}

func addDir(ctx context.Context, tw *tar.Writer, baseDir, rel string) error {
	root := filepath.Join(baseDir, rel)
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		if skipArchivePath(relPath) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return addFile(tw, baseDir, relPath, info)
	})
}

func addFile(tw *tar.Writer, baseDir, rel string, info os.FileInfo) error {
	path := filepath.Join(baseDir, rel)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// 必须以「打开后」的尺寸为准：WalkDir 给的 info 是遍历那一刻的快照，
	// 而这台机器上 logs、frpc 配置这些文件随时在长。用旧尺寸写进 tar 头、
	// 再读出更多内容，就会报 archive/tar: write too long。
	size := info.Size()
	if current, err := file.Stat(); err == nil {
		size = current.Size()
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(rel)
	header.Size = size
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	// 精确写入 size 字节：tar 要求实际字节数与头部声明严格相等。
	//   - 文件在读取期间变长 → 多余部分截断（LimitReader）；
	//   - 文件在读取期间被截短 → 补零凑够（zeroReader），保证这一条记录结构完整，
	//     不至于让整份归档作废。
	written, err := io.Copy(tw, io.LimitReader(file, size))
	if err != nil {
		return err
	}
	if written < size {
		if _, err := io.CopyN(tw, zeroReader{}, size-written); err != nil {
			return err
		}
	}
	return nil
}

// zeroReader 提供无限个零字节，用于补齐读取期间被截短的文件条目。
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
