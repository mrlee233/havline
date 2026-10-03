//go:build !linux && !darwin && !freebsd && !windows

package frp

import "os"

func probeProcess(pid int) (bool, string) {
	if pid <= 0 {
		return false, ""
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, ""
	}
	return true, process.String()
}
