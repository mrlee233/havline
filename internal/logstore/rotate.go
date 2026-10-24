package logstore

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Nginx 自己不会轮转日志，长期运行的 NAS 上 access.log 会一直长。
// 这里的策略刻意保持简单：
//   - 超过 MaxBytes 才轮转，避免一天切一堆小文件；
//   - 轮转时把 access.log 改名成 access-<时间戳>.log.gz，再让 Nginx 重开文件；
//   - 保留最近 Keep 份，更旧的删掉。
//
// 用「rename + 信号重开」而不是 truncate：truncate 会丢掉读侧（流量采集）还没读到的内容。
const (
	// DefaultMaxBytes 单个日志文件的上限（200MB）。NAS 上反代日志增长很快，
	// 太大则读取接口变慢，太小则轮转频繁、压缩开销占比升高。
	DefaultMaxBytes int64 = 200 << 20
	// DefaultKeep 每个日志保留的归档份数。
	DefaultKeep = 5
)

// Rotator 负责单个日志文件的轮转与清理。
type Rotator struct {
	// Path 当前正在写入的日志路径（例如 <DataDir>/logs/access.log）
	Path string
	// MaxBytes 超过它就轮转；<= 0 用 DefaultMaxBytes
	MaxBytes int64
	// Keep 保留份数；<= 0 用 DefaultKeep
	Keep int
	// MaxAgeDays 归档保留天数；<= 0 表示不按时间清理（只按份数）。
	// 对应「设置 → 常规 → 日志保留天数」，填了才生效。
	MaxAgeDays int
	// reopen 通知写日志的进程重新打开文件（Nginx 用 USR1）
	reopen func() error
}

// NewRotator 装配一个日志轮转器。reopen 用于通知写日志的进程重新打开文件
// （Nginx 用 USR1）；传 nil 表示该日志由本进程自己写，不需要重开。
func NewRotator(path string, maxAgeDays int, reopen func() error) *Rotator {
	return &Rotator{Path: path, MaxAgeDays: maxAgeDays, reopen: reopen}
}

// RotateIfNeeded 在文件超过阈值时轮转，返回是否真的轮转了。
// 未超阈值时不产生任何副作用，可以安全地高频调用。
func (r *Rotator) RotateIfNeeded(now time.Time) (bool, error) {
	info, err := os.Stat(r.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	limit := r.MaxBytes
	if limit <= 0 {
		limit = DefaultMaxBytes
	}
	if info.Size() < limit {
		return false, nil
	}

	target := r.archivePath(now)
	if err := compressRename(r.Path, target); err != nil {
		return false, err
	}
	// 文件被改名后，Nginx 仍持有旧 inode 继续往里写。必须让它重开，
	// 否则新内容会写进已归档的那份文件，轮转等于没做。
	if r.reopen != nil {
		if err := r.reopen(); err != nil {
			return true, fmt.Errorf("日志已归档，但通知重开失败：%w", err)
		}
	}
	// 归档成功后再清理旧份数：清理失败不该让本轮轮转算失败
	r.prune()
	return true, nil
}

// archivePath 生成归档名：access.log → access-20261024-1305.log.gz。
// 时间戳精确到分钟，同一分钟内重复轮转时补序号，避免覆盖。
func (r *Rotator) archivePath(now time.Time) string {
	dir := filepath.Dir(r.Path)
	base := strings.TrimSuffix(filepath.Base(r.Path), ".log")
	stamp := now.Format("20060102-1504")
	candidate := filepath.Join(dir, fmt.Sprintf("%s-%s.log.gz", base, stamp))
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.log.gz", base, stamp, i))
	}
}

// prune 先按保留天数删超期归档，再按份数保留最近的。
// 两个条件都生效：天数保证「长期不轮转也不会无限留」，份数保证「高频轮转不会堆积」。
func (r *Rotator) prune() {
	archives := r.archives()
	if r.MaxAgeDays > 0 {
		cutoff := time.Now().Add(-time.Duration(r.MaxAgeDays) * 24 * time.Hour)
		kept := archives[:0]
		for _, path := range archives {
			info, err := os.Stat(path)
			if err == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(path)
				continue
			}
			kept = append(kept, path)
		}
		archives = kept
	}
	keep := r.Keep
	if keep <= 0 {
		keep = DefaultKeep
	}
	for i := 0; i < len(archives)-keep; i++ {
		_ = os.Remove(archives[i])
	}
}

// archives 列出该日志的归档文件，按旧→新排序。
func (r *Rotator) archives() []string {
	dir := filepath.Dir(r.Path)
	base := strings.TrimSuffix(filepath.Base(r.Path), ".log")
	prefix := base + "-"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log.gz") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out
}

// compressRename 把 src 压缩写到 dst，成功后删除 src。
// 先写 .tmp 再改名：压缩中途失败不会留下一个看起来能用的归档。
func compressRename(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		_ = in.Close()
		return err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(tmp)
		return err
	}
	// 关闭顺序与错误都要检查：gzip 的尾部写在 Close 里，漏了就是半截归档
	if err := gz.Close(); err != nil {
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = in.Close()
		_ = os.Remove(tmp)
		return err
	}
	// 必须显式关闭源文件再删：Windows 上句柄没放掉时 Remove/改名都会失败
	if err := in.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// 归档成功才删原文件；顺序反了会在压缩失败时丢日志
	return os.Remove(src)
}
