package updater

import (
	"fmt"
	"strconv"
	"strings"
)

type Phase string

const (
	PhaseIdle      Phase = "idle"
	PhaseScheduled Phase = "scheduled"
	PhaseRunning   Phase = "running"
	PhaseSuccess   Phase = "success"
	PhaseFailed    Phase = "failed"
)

// Status 是主程序与 havline-updater sidecar 共享的更新状态。
type Status struct {
	Enabled         bool     `json:"enabled"`
	Mode            string   `json:"mode"`
	CurrentVersion  string   `json:"current_version,omitempty"`
	LatestVersion   string   `json:"latest_version,omitempty"`
	UpdateAvailable bool     `json:"update_available"`
	Busy            bool     `json:"busy"`
	Phase           Phase    `json:"phase"`
	Message         string   `json:"message"`
	Log             []string `json:"log,omitempty"`
	CheckedAt       string   `json:"checked_at,omitempty"`
}

func busyPhase(phase Phase) bool {
	return phase == PhaseScheduled || phase == PhaseRunning
}

// CompareVersions 比较形如 1.2.3、v1.2.3 的版本号；返回值大于 0 表示 a 更新。
func CompareVersions(a, b string) int {
	left := parseVersion(a)
	right := parseVersion(b)
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		var lv, rv int
		if index < len(left) {
			lv = left[index]
		}
		if index < len(right) {
			rv = right[index]
		}
		if lv != rv {
			if lv > rv {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseVersion(value string) []int {
	clean := strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if index := strings.IndexAny(clean, "-+"); index >= 0 {
		clean = clean[:index]
	}
	parts := strings.Split(clean, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, number)
	}
	return out
}

func validateSemver(value string) error {
	if value == "" {
		return fmt.Errorf("版本号为空")
	}
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) != 3 {
		return fmt.Errorf("版本号格式无效: %s", value)
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return fmt.Errorf("版本号格式无效: %s", value)
		}
	}
	return nil
}
