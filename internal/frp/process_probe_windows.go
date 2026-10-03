//go:build windows

package frp

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func probeProcess(pid int) (bool, string) {
	if pid <= 0 {
		return false, ""
	}
	script := fmt.Sprintf("$p=Get-CimInstance Win32_Process -Filter \"ProcessId=%s\"; if ($p) { $p.CommandLine }", strconv.Itoa(pid))
	output, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return false, ""
	}
	return strings.TrimSpace(string(output)) != "", strings.TrimSpace(string(output))
}
