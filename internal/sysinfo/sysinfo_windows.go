//go:build windows

package sysinfo

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceExW  = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func readPlatform() (Stats, CPUSample) {
	var (
		stats  Stats
		sample CPUSample
	)

	var idle, kernel, user syscall.Filetime
	if ret, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); ret != 0 {
		// kernel 时间包含 idle，因此总时间 = kernel + user
		sample = CPUSample{
			Total: filetimeTicks(kernel) + filetimeTicks(user),
			Idle:  filetimeTicks(idle),
		}
	}

	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	if ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); ret != 0 {
		stats.MemTotalBytes = mem.TotalPhys
		stats.MemUsedBytes = mem.TotalPhys - mem.AvailPhys
	}

	stats.DiskTotalBytes, stats.DiskUsedBytes = diskUsage(volumeRoot())
	return stats, sample
}

// volumeRoot 取当前工作目录所在盘符（Havline 的数据目录与工作目录同盘），失败时回落到 C:\
func volumeRoot() string {
	wd, err := os.Getwd()
	if err != nil || len(wd) < 2 || wd[1] != ':' {
		return `C:\`
	}
	return wd[:3]
}

func diskUsage(root string) (total, used uint64) {
	rootPtr, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0
	}
	var freeToCaller, totalBytes, totalFree uint64
	ret, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret == 0 || totalBytes == 0 {
		return 0, 0
	}
	if totalFree > totalBytes {
		return totalBytes, 0
	}
	return totalBytes, totalBytes - totalFree
}

// LoadAvg Windows 没有平均负载概念，固定返回 nil
func LoadAvg() []float64 {
	return nil
}

// UptimeSeconds 用 GetTickCount64 取系统运行秒数
func UptimeSeconds() uint64 {
	ret, _, _ := procGetTickCount64.Call()
	return uint64(ret) / 1000
}

// filetimeTicks 把 FILETIME（100ns 单位）折算为 10ms 刻度，避免累加时溢出
func filetimeTicks(ft syscall.Filetime) uint64 {
	return (uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)) / 10000
}
