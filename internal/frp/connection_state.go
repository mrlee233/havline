package frp

import (
	"os"
	"strings"
)

const maxConnectionLogRead = 128 << 10

func inspectConnectionLog(path, processState string) (string, string) {
	if processState != "running" {
		if processState == "error" {
			return "error", "frpc 进程已退出"
		}
		return "disconnected", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown", ""
	}
	if len(data) > maxConnectionLogRead {
		data = data[len(data)-maxConnectionLogRead:]
	}
	lines := strings.Split(string(data), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "login to server success"):
			return "connected", ""
		case strings.Contains(lower, "login to server failed"), strings.Contains(lower, "connect to server error"):
			return "error", trimConnectionError(line)
		case strings.Contains(lower, "frpc service") && strings.Contains(lower, "stopped"):
			return "disconnected", ""
		}
	}
	return "unknown", ""
}

func trimConnectionError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
