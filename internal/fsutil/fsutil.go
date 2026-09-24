// Package fsutil 收敛「原子写配置」与「覆盖前备份」两个到处重复的动作。
//
// 之前 agent、frp、nginx、chinacidr 各自实现了一遍：有的用固定 .tmp 名（并发写同一路径会互相踩），
// 有的直接 os.WriteFile（读到写了一半的配置），有的写完不 chmod（权限跟着 umask 走）。
package fsutil

import (
	"os"
	"path/filepath"
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
