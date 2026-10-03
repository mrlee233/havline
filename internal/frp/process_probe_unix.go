//go:build linux || darwin || freebsd

package frp

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func probeProcess(pid int) (bool, string) {
	if pid <= 0 {
		return false, ""
	}
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline"); err == nil {
		commandLine := strings.ReplaceAll(string(data), "\x00", " ")
		return strings.TrimSpace(commandLine) != "", strings.TrimSpace(commandLine)
	}
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false, ""
	}
	return strings.TrimSpace(string(output)) != "", strings.TrimSpace(string(output))
}
