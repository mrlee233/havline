//go:build !linux && !windows

package sysinfo

// readPlatform 在未适配的平台（macOS / BSD 等）上返回空快照，前端按「不可用」处理
func readPlatform() (Stats, CPUSample) {
	return Stats{}, CPUSample{}
}

// LoadAvg 未适配的平台没有平均负载概念
func LoadAvg() []float64 {
	return nil
}

// UptimeSeconds 未适配的平台不采集运行时长
func UptimeSeconds() uint64 {
	return 0
}
