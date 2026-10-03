// Package sysinfo 采集主机资源占用（CPU / 内存 / 磁盘），供仪表盘展示。
//
// 注意：在容器（Docker）里运行时，/proc 与根文件系统通常反映的是宿主机的总量与占用
// （除非挂载了 lxcfs 之类的视图隔离），因此这里的数值是「宿主机视角」，不是容器配额。
package sysinfo

import (
	"strconv"
	"strings"
)

// CPUSample 是 CPU 累计计数器的一次快照；占用率需要两次采样求差值
type CPUSample struct {
	Total uint64
	Idle  uint64
}

// Stats 是主机资源快照；当前平台采集不到的字段保持 0
type Stats struct {
	CPUPercent     float64
	MemUsedBytes   uint64
	MemTotalBytes  uint64
	DiskUsedBytes  uint64
	DiskTotalBytes uint64
}

// Read 采集一次主机资源。cpuPrev 为上次采样的 CPU 计数器（零值表示首次采集，
// 此时 CPUPercent 为 0），返回的 sample 供下次调用传入。
func Read(cpuPrev CPUSample) (Stats, CPUSample) {
	stats, sample := readPlatform()
	stats.CPUPercent = CPUPercent(cpuPrev, sample)
	return stats, sample
}

// CPUPercent 由两次采样的差值计算占用率；首次采样或计数器未前进时返回 0
func CPUPercent(prev, cur CPUSample) float64 {
	if prev.Total == 0 || cur.Total <= prev.Total {
		return 0
	}
	totalDelta := cur.Total - prev.Total
	var idleDelta uint64
	if cur.Idle > prev.Idle {
		idleDelta = cur.Idle - prev.Idle
	}
	if idleDelta >= totalDelta {
		return 0
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100
}

// parseProcStat 解析 /proc/stat 的 cpu 汇总行；第 4、5 列（idle、iowait）都计入空闲
func parseProcStat(raw string) CPUSample {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var total, idle uint64
		for i, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return CPUSample{}
			}
			total += value
			if i == 3 || i == 4 {
				idle += value
			}
		}
		return CPUSample{Total: total, Idle: idle}
	}
	return CPUSample{}
}

// parseMemInfo 解析 /proc/meminfo（值以 kB 为单位），返回总内存与已用内存字节数。
// 已用内存优先按 MemAvailable 计算，缺失时回落到 MemFree + Buffers + Cached。
func parseMemInfo(raw string) (total, used uint64) {
	values := make(map[string]uint64, 16)
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		amount, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSpace(key)] = amount * 1024
	}
	total = values["MemTotal"]
	if total == 0 {
		return 0, 0
	}
	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if available > total {
		return total, 0
	}
	return total, total - available
}
