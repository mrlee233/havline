//go:build linux

package sysinfo

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func readPlatform() (Stats, CPUSample) {
	var (
		stats  Stats
		sample CPUSample
	)
	if raw, err := os.ReadFile("/proc/stat"); err == nil {
		sample = parseProcStat(string(raw))
	}
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		stats.MemTotalBytes, stats.MemUsedBytes = parseMemInfo(string(raw))
	}
	stats.DiskTotalBytes, stats.DiskUsedBytes = diskUsage("/")
	return stats, sample
}

// LoadAvg 返回 1 / 5 / 15 分钟平均负载；读取失败返回 nil
func LoadAvg() []float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 3 {
		return nil
	}
	values := make([]float64, 0, 3)
	for _, field := range fields[:3] {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return nil
		}
		values = append(values, value)
	}
	return values
}

// UptimeSeconds 返回系统运行秒数（/proc/uptime）
func UptimeSeconds() uint64 {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return uint64(seconds)
}

func diskUsage(path string) (total, used uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if free > total {
		return total, 0
	}
	return total, total - free
}
