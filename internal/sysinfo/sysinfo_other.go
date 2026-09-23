//go:build !linux && !windows

package sysinfo

// readPlatform 在未适配的平台（macOS / BSD 等）上返回空快照，前端按「不可用」处理
func readPlatform() (Stats, CPUSample) {
	return Stats{}, CPUSample{}
}
