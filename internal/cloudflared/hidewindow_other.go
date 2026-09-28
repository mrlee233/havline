//go:build !windows

package cloudflared

import "syscall"

func hideWindowAttr() *syscall.SysProcAttr {
	return nil
}
