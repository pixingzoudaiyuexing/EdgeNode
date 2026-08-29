//go:build plus

package nodes

import (
	"github.com/TeaOSLab/EdgeNode/internal/utils/fasttime"
	maputils "github.com/TeaOSLab/EdgeNode/internal/utils/maps"
)

const (
	// 可信 1.3.9 Plus ELF 的 .data 中可直接读取到原版 FixedMap.maxSize=65535。
	// 对应全局指针 0x52f3198 -> 静态对象 0x5311b60，FixedMap 的 maxSize 位于 +32。
	ccBlockedCounterMapMaxSize = 65535
	ccBlockedCounterLife       = int64(24 * 60 * 60)
	ccBlockedCounterMax        = int32(32)
)

// ccBlockedCounter 与可信 1.3.9 Plus Go 类型信息一致：只有 count 和 updatedAt 两个字段。
// 它记录同一客户端 IP 在 24 小时内重复触发 CC 封禁的次数，用于线性放大封禁时长。
type ccBlockedCounter struct {
	count     int32
	updatedAt int64
}

var ccBlockedCounterMap = newCCBlockedCounterMap()

func newCCBlockedCounterMap() *maputils.FixedMap[string, *ccBlockedCounter] {
	return maputils.NewFixedMap[string, *ccBlockedCounter](ccBlockedCounterMapMaxSize)
}

// increaseCCCounter 恢复 1.3.9 Plus 的重复 CC 封禁倍率状态。
//
// 原版行为已经由可信二进制的函数边界、类型信息和汇编交叉确认：
//   - key 只使用客户端 IP，不包含 serverId 或 URL；
//   - 距上一次触发超过 86400 秒时从 1 重新开始；
//   - 24 小时内每次触发 +1；
//   - 最高 32；
//   - 返回值直接作为 BlockSeconds 的线性倍率。
func (this *HTTPRequest) increaseCCCounter(remoteAddr string) int32 {
	now := fasttime.Now().Unix()
	counter, ok := ccBlockedCounterMap.Get(remoteAddr)
	if ok {
		if counter.updatedAt < now-ccBlockedCounterLife {
			counter.count = 0
		}
		counter.updatedAt = now
		if counter.count < ccBlockedCounterMax {
			counter.count++
		}
		return counter.count
	}

	counter = &ccBlockedCounter{
		count:     1,
		updatedAt: now,
	}
	ccBlockedCounterMap.Put(remoteAddr, counter)
	return counter.count
}
