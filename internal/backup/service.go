package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	dataDir string
}

// 还原时的固定权限：备份内容只有数据库、证书与 nginx 配置，不接受归档自带的权限位
const (
	restoreFileMode = 0o640
	restoreDirMode  = 0o750
)

func New(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

// backupTimeFmt 归档文件名里的时间戳；List / Path 都按同一套格式校验
const backupTimeFmt = "20060102-150405"

// Export 导出一份归档并返回路径；文件留在 <DataDir>/backups 下，由调用方决定下载后删除还是长期保留。
func (s *Service) Export(ctx context.Context) (string, error) {
	if err := os.MkdirAll(s.backupsDir(), 0o755); err != nil {
		return "", err
	}
	// CreateTemp 保证同一秒内多次导出不会覆盖已有备份（文件名带随机后缀）
	file, err := os.CreateTemp(s.backupsDir(), fmt.Sprintf("havline-backup-%s-*.tar.gz", time.Now().Format(backupTimeFmt)))
	if err != nil {
		return "", err
	}
	outPath := file.Name()
	if err := s.writeArchive(ctx, file, outPath); err != nil {
		return "", err
	}
	return outPath, nil
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
	for _, entry := range []string{"havline.db", "certs", "nginx", "cloudflared/tunnels"} {
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

func (s *Service) Restore(ctx context.Context, archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
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
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, restoreFileMode)
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

func addDir(ctx context.Context, tw *tar.Writer, baseDir, rel string) error {
	root := filepath.Join(baseDir, rel)
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.Contains(path, string(os.PathSeparator)+"backups"+string(os.PathSeparator)) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(baseDir, path)
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
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(rel)
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	return err
}
