package sysinfo

import "testing"

func TestCPUPercent(t *testing.T) {
	prev := CPUSample{Total: 1000, Idle: 400}
	if got := CPUPercent(prev, CPUSample{Total: 1200, Idle: 500}); got != 50 {
		t.Fatalf("expected 50%%, got %v", got)
	}
	// 首次采样（prev 为零值）不给出占用率
	if got := CPUPercent(CPUSample{}, CPUSample{Total: 1200, Idle: 500}); got != 0 {
		t.Fatalf("expected 0 on first sample, got %v", got)
	}
	// 计数器未前进
	if got := CPUPercent(prev, prev); got != 0 {
		t.Fatalf("expected 0 when counters stalled, got %v", got)
	}
}

func TestParseProcStat(t *testing.T) {
	raw := "cpu  100 20 80 400 10 0 5 0 0 0\ncpu0 1 2 3 4 5 6 7 8 9 10\n"
	sample := parseProcStat(raw)
	// 总时间 100+20+80+400+10+0+5 = 615，空闲 400(idle)+10(iowait) = 410
	if sample.Total != 615 || sample.Idle != 410 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
}

func TestParseMemInfo(t *testing.T) {
	raw := "MemTotal:       16384000 kB\nMemFree:         1000000 kB\nMemAvailable:    8192000 kB\n"
	total, used := parseMemInfo(raw)
	if total != 16384000*1024 {
		t.Fatalf("unexpected total: %d", total)
	}
	if used != (16384000-8192000)*1024 {
		t.Fatalf("unexpected used: %d", used)
	}

	// 缺少 MemAvailable 时回落到 MemFree + Buffers + Cached
	fallback := "MemTotal:       1000 kB\nMemFree:         200 kB\nBuffers:         100 kB\nCached:          300 kB\n"
	total, used = parseMemInfo(fallback)
	if total != 1000*1024 || used != 400*1024 {
		t.Fatalf("unexpected fallback: total=%d used=%d", total, used)
	}
}

func TestReadReportsHostResources(t *testing.T) {
	stats, sample := Read(CPUSample{})
	if stats.MemTotalBytes == 0 && stats.DiskTotalBytes == 0 {
		t.Skip("当前平台未实现资源采集")
	}
	if stats.MemTotalBytes == 0 || stats.MemUsedBytes > stats.MemTotalBytes {
		t.Fatalf("unexpected memory: used=%d total=%d", stats.MemUsedBytes, stats.MemTotalBytes)
	}
	if stats.DiskTotalBytes == 0 || stats.DiskUsedBytes > stats.DiskTotalBytes {
		t.Fatalf("unexpected disk: used=%d total=%d", stats.DiskUsedBytes, stats.DiskTotalBytes)
	}
	if sample.Total == 0 {
		t.Fatalf("expected non-zero cpu counters")
	}
}
