//go:build !linux

package sysinfo

import "runtime"

func readSysMem() (total uint64, used uint64, percent float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// 非 Linux 平台使用进程内存占用进行基础模拟
	total = 4 * 1024 * 1024 * 1024 // 4GB 预设
	used = m.Sys
	percent = float64(used) / float64(total) * 100
	return
}

func readCPUPercent() float64 {
	return 0.5 // 基础静态
}
