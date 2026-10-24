//go:build linux || darwin || freebsd

package nginx

import (
	"os"
	"syscall"
)

func reloadProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGHUP)
}

// reopenLogsProcess 让 Nginx 重新打开日志文件（USR1）。
//
// 日志轮转必须用信号重开、而不是 reload 配置：reload 会拉起新 worker 并优雅退出旧
// worker，代价与风险都远大于「关掉旧 fd、按同路径重新 open」这一件事。
func reopenLogsProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGUSR1)
}

func terminateProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

func isPIDAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
