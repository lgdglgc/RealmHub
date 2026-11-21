//go:build linux

package sysinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

var (
	lastIdleTime  uint64
	lastTotalTime uint64
)

func readSysMem() (total uint64, used uint64, percent float64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		valBytes := val * 1024 // kB -> Bytes

		switch key {
		case "MemTotal":
			memTotal = valBytes
		case "MemAvailable":
			memAvailable = valBytes
		}
	}

	if memTotal > 0 {
		total = memTotal
		used = memTotal - memAvailable
		percent = float64(used) / float64(memTotal) * 100
	}
	return
}

func readCPUPercent() float64 {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0
	}
	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	var total uint64
	var idle uint64
	for i, f := range fields[1:] {
		val, _ := strconv.ParseUint(f, 10, 64)
		total += val
		if i == 3 { // idle 是第 4 个字段
			idle = val
		}
	}

	if lastTotalTime == 0 {
		lastIdleTime = idle
		lastTotalTime = total
		return 1.0
	}

	totalDelta := total - lastTotalTime
	idleDelta := idle - lastIdleTime
	lastIdleTime = idle
	lastTotalTime = total

	if totalDelta == 0 {
		return 0
	}

	used := totalDelta - idleDelta
	return float64(used) / float64(totalDelta) * 100
}
