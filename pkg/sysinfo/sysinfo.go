package sysinfo

import (
	"os"
	"runtime"
	"time"
)

var startTime = time.Now()

// Metrics 系统指标
type Metrics struct {
	OS        string  `json:"os"`
	Arch      string  `json:"arch"`
	Hostname  string  `json:"hostname"`
	CPUUsage  float64 `json:"cpu_usage"`
	MemUsage  float64 `json:"mem_usage"`
	MemTotal  uint64  `json:"mem_total"`
	MemUsed   uint64  `json:"mem_used"`
	UptimeSec uint64  `json:"uptime_sec"`
}

// GetMetrics 获取系统基本运行指标（跨平台轻量采集，无第三方 CGO 依赖）
func GetMetrics() Metrics {
	hostname, _ := os.Hostname()
	uptime := uint64(time.Since(startTime).Seconds())

	// 内存指标
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 基础轻量估算（在实际 Linux 机器上可通过 /proc/meminfo 和 /proc/stat 进一步精确）
	memTotal, memUsed, memPercent := readSysMem()
	cpuPercent := readCPUPercent()

	return Metrics{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Hostname:  hostname,
		CPUUsage:  cpuPercent,
		MemUsage:  memPercent,
		MemTotal:  memTotal,
		MemUsed:   memUsed,
		UptimeSec: uptime,
	}
}
