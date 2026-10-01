package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/havline/havline/internal/settings"
)

// 保留份数的边界：0/负数按默认值处理，上限防止把磁盘塞满
const (
	DefaultKeepCount = 7
	MaxKeepCount     = 60
)

// archiveNamePattern 备份命名格式（Export 用 CreateTemp 生成，后缀是随机数字）。
// List / Path 都按它过滤，避免把 backups 目录里任意文件当成备份读写或删除。
var archiveNamePattern = regexp.MustCompile(`^havline-backup-\d{8}-\d{6}-\d+\.tar\.gz$`)

// Archive 留在磁盘上的一份备份
type Archive struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

func (s *Service) backupsDir() string {
	return filepath.Join(s.dataDir, "backups")
}

// List 按新的在前列出备份；目录不存在返回空列表（还没备份过是正常状态）。
func (s *Service) List() ([]Archive, error) {
	entries, err := os.ReadDir(s.backupsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	archives := make([]Archive, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !archiveNamePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		archives = append(archives, Archive{Name: entry.Name(), Size: info.Size(), ModTime: info.ModTime().UTC()})
	}
	// 文件名前缀就是时间戳，按名倒序即按时间倒序
	sort.Slice(archives, func(i, j int) bool { return archives[i].Name > archives[j].Name })
	return archives, nil
}

// Prune 只保留最新的 keep 份，返回删除数量；keep <= 0 表示不剪枝。
func (s *Service) Prune(keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}
	archives, err := s.List()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, archive := range archives[min(keep, len(archives)):] {
		if err := s.Remove(archive.Name); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Path 把备份名解析成绝对路径；名字必须匹配备份命名格式，避免被用来读写目录外的文件。
func (s *Service) Path(name string) (string, error) {
	if !archiveNamePattern.MatchString(name) {
		return "", fmt.Errorf("备份名无效")
	}
	return filepath.Join(s.backupsDir(), name), nil
}

// Remove 删除指定备份
func (s *Service) Remove(name string) error {
	path, err := s.Path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("备份不存在")
		}
		return err
	}
	return nil
}

// Verify 回读归档并返回条目数：确认这份备份不是写了一半的残件，
// 同时要求包内必须有 havline.db（否则多半不是 Havline 的备份）。
func (s *Service) Verify(ctx context.Context, path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("备份文件无法解压：%w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	count := 0
	hasDatabase := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("备份文件已损坏：%w", err)
		}
		if err := ctx.Err(); err != nil {
			return count, err
		}
		count++
		if header.Name == "havline.db" {
			hasDatabase = true
		}
	}
	if !hasDatabase {
		return count, fmt.Errorf("备份里没有 havline.db，可能不是 Havline 的备份")
	}
	return count, nil
}

// KeepCount 读取保留份数（未配置或越界时回落到默认值）。
// 自动备份任务与「立即备份」共用它，避免两处各写一套默认值。
func KeepCount(ctx context.Context, store *settings.Store) int {
	keep, err := store.GetInt(ctx, settings.KeyBackupKeepCount)
	if err != nil || keep <= 0 {
		return DefaultKeepCount
	}
	if keep > MaxKeepCount {
		return MaxKeepCount
	}
	return keep
}
