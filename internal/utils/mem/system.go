package memutils

import (
	teaconst "github.com/TeaOSLab/EdgeNode/internal/const"
	"github.com/TeaOSLab/EdgeNode/internal/utils/goman"
	gopsutilmem "github.com/shirou/gopsutil/v3/mem"
	"time"
)

var systemTotalMemory = -1
var systemMemoryBytes uint64
var availableMemoryGB int

func init() {
	if !teaconst.IsMain {
		return
	}

	_ = SystemMemoryGB()

	// 与 1.3.9 节点保持一致：后台周期刷新当前可用内存，供运行时策略读取。
	goman.New(func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			stat, err := gopsutilmem.VirtualMemory()
			if err == nil {
				availableMemoryGB = int(stat.Available >> 30)
			}
		}
	})
}

// SystemMemoryGB 返回系统总内存的 GiB 整数值。
// 原版要求该值至少为 1；探测失败时同样回退到 1。
func SystemMemoryGB() int {
	if systemTotalMemory > 0 {
		return systemTotalMemory
	}

	stat, err := gopsutilmem.VirtualMemory()
	if err != nil {
		return 1
	}

	systemMemoryBytes = stat.Total
	availableMemoryGB = int(stat.Available >> 30)
	systemTotalMemory = int(stat.Total >> 30)
	if systemTotalMemory <= 0 {
		systemTotalMemory = 1
	}

	setMaxMemory(systemTotalMemory)
	return systemTotalMemory
}

// SystemMemoryBytes 返回最近一次成功探测到的系统总内存字节数。
func SystemMemoryBytes() uint64 {
	return systemMemoryBytes
}

// AvailableMemoryGB 返回最近一次探测到的可用内存 GiB 整数值。
func AvailableMemoryGB() int {
	return availableMemoryGB
}
