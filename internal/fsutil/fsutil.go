// Package fsutil 收敛「原子写配置」与「覆盖前备份」两个到处重复的动作。
//
// 之前 agent、frp、nginx、chinacidr 各自实现了一遍：有的用固定 .tmp 名（并发写同一路径会互相踩），
// 有的直接 os.WriteFile（读到写了一半的配置），有的写完不 chmod（权限跟着 umask 走）。
package fsutil

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// WriteFileAtomic 先写同目录的临时文件再 rename：读者要么看到旧内容、要么看到新内容。
// 临时文件名带随机后缀，避免并发写同一路径时互相覆盖。
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp 建出来的是 0600，这里显式对齐调用方要求的权限
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = "" // rename 成功，不用再清理
	return nil
}

// Backup 把现有文件复制成 <path>.bak；文件不存在视为无需备份。
// 返回的错误必须由调用方处理——备份是「覆盖前留退路」的唯一保障。
func Backup(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return WriteFileAtomic(path+".bak", data, 0o600)
}

// VersionsDir 返回某文件的版本目录（.<name>.versions），供列表 / 回滚读取。
// 目录名以点号开头，避免被 nginx 的 include <dir>/* 当成配置文件读取。
func VersionsDir(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".versions")
}

// legacyVersionsDir 是旧布局（<path>.versions）。它会被 nginx 的 include * 误读为配置文件，
// 因此读取 / 备份前统一迁移到隐藏目录。
func legacyVersionsDir(path string) string {
	return path + ".versions"
}

// MigrateLegacyVersions 把旧版本目录迁移到隐藏目录。只移动内容，不删除历史版本；
// 迁移后旧目录已空，删除它是为了让 nginx 的 include * 不再命中该目录。
func MigrateLegacyVersions(path string) error {
	legacy := legacyVersionsDir(path)
	info, err := os.Stat(legacy)
	if err != nil || !info.IsDir() {
		return nil
	}
	target := VersionsDir(path)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Rename(legacy, target); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(legacy, entry.Name())
		dst := filepath.Join(target, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return os.Remove(legacy)
}

// BackupVersioned 把现有文件留存为 .<name>.versions/<时间戳>-<随机>.conf，并只保留最近 keep 份。
// 文件名前缀是 UTC 时间戳（同一秒内用随机后缀避开冲突），因此目录内按文件名倒序就是版本新旧顺序。
// 文件不存在视为无需备份，返回空串；keep <= 0 表示不剪枝。
func BackupVersioned(path string, keep int) (string, error) {
	if err := MigrateLegacyVersions(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dir := VersionsDir(path)
	// 内容与最新版本一致时不重复记录：Apply 每次保存都会调用，去重后保留期才对应「真实的 N 次变更」
	if versions, err := ListVersions(path); err == nil && len(versions) > 0 {
		if newest, err := ReadVersion(path, versions[0].Name); err == nil && bytes.Equal(newest, data) {
			return filepath.Join(dir, versions[0].Name), nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := time.Now().UTC().Format("20060102T150405.000") + "-" + randSuffix() + filepath.Ext(path)
	target := filepath.Join(dir, name)
	if err := WriteFileAtomic(target, data, 0o600); err != nil {
		return "", err
	}
	if keep > 0 {
		pruneVersions(dir, keep)
	}
	return target, nil
}

// VersionEntry 描述一个已留存的版本（名称前缀是 UTC 时间戳，因此按名称倒序就是新旧顺序）。
type VersionEntry struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// ListVersions 按新的在前列出版本；版本目录不存在时返回空列表（首次备份前是正常状态）。
func ListVersions(path string) ([]VersionEntry, error) {
	if err := MigrateLegacyVersions(path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(VersionsDir(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]VersionEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, VersionEntry{Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// ReadVersion 读取指定版本的内容。版本名虽然来自 ListVersions，这里仍校验一次：
// 带路径分隔符 / 上跳的名称直接拒绍，避免接口被用来读版本目录以外的文件。
func ReadVersion(path, name string) ([]byte, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return nil, fmt.Errorf("版本名无效")
	}
	if err := MigrateLegacyVersions(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(VersionsDir(path), name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("版本不存在")
		}
		return nil, err
	}
	return data, nil
}

// pruneVersions 只保留最新的 keep 份（按文件名倒序，前缀是时间戳）。
// 剪枝失败不影响本次备份结果，因此只忽略错误。
func pruneVersions(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[min(keep, len(names)):] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func randSuffix() string {
	var buf [2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "0000"
	}
	return hex.EncodeToString(buf[:])
}
