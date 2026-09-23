//go:build linux

package sysinfo

import (
	"os"
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
