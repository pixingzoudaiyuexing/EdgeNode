package mem

import gopsutilmem "github.com/shirou/gopsutil/v3/mem"

var systemTotalMemoryGB = -1

func init() {
	_ = SystemMemoryGB()
}

// SystemMemoryGB 返回物理内存总量的 GiB 整数部分，并缓存首次成功结果。
// 该换算方式与 GoEdge 1.3.9 同代实现保持一致。
func SystemMemoryGB() int {
	if systemTotalMemoryGB > 0 {
		return systemTotalMemoryGB
	}
	stat, err := gopsutilmem.VirtualMemory()
	if err != nil {
		return 0
	}
	systemTotalMemoryGB = int(stat.Total / 1024 / 1024 / 1024)
	return systemTotalMemoryGB
}
