//go:build plus

package nodes

import (
	"testing"

	maputils "github.com/TeaOSLab/EdgeNode/internal/utils/maps"
)

func TestIncreaseCCCounterStartsAtOneAndCapsAt32(t *testing.T) {
	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()

	req := &HTTPRequest{}
	const ip = "198.51.100.80"
	if got := req.increaseCCCounter(ip); got != 1 {
		t.Fatalf("第一次 CC 封禁倍率应为 1，实际 %d", got)
	}
	for want := int32(2); want <= ccBlockedCounterMax; want++ {
		if got := req.increaseCCCounter(ip); got != want {
			t.Fatalf("第 %d 次 CC 封禁倍率应为 %d，实际 %d", want, want, got)
		}
	}
	if got := req.increaseCCCounter(ip); got != ccBlockedCounterMax {
		t.Fatalf("超过上限后倍率应保持 %d，实际 %d", ccBlockedCounterMax, got)
	}
}

func TestIncreaseCCCounterResetsAfter24Hours(t *testing.T) {
	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()

	const ip = "203.0.113.81"
	// 直接构造已过期状态，避免测试依赖真实等待时间。
	ccBlockedCounterMap.Put(ip, &ccBlockedCounter{
		count:     9,
		updatedAt: 1,
	})

	req := &HTTPRequest{}
	if got := req.increaseCCCounter(ip); got != 1 {
		t.Fatalf("超过 24 小时后倍率应重置为 1，实际 %d", got)
	}
}

func TestCCBlockedCounterMapCapacityMatchesOriginal(t *testing.T) {
	m := newCCBlockedCounterMap()
	for i := 0; i <= ccBlockedCounterMapMaxSize; i++ {
		m.Put(iToCCCounterKey(i), &ccBlockedCounter{count: 1})
	}
	if got := len(m.RawMap()); got != ccBlockedCounterMapMaxSize {
		t.Fatalf("CC FixedMap 容量应为 %d，实际 %d", ccBlockedCounterMapMaxSize, got)
	}
	if m.Has(iToCCCounterKey(0)) {
		t.Fatal("容量溢出后原版 FixedMap 应淘汰最早插入的 key")
	}
}

func iToCCCounterKey(v int) string {
	// 不引入 strconv 之外的状态；测试 key 只需稳定且唯一。
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for v > 0 {
		buf = append(buf, digits[v&0xf])
		v >>= 4
	}
	for left, right := 0, len(buf)-1; left < right; left, right = left+1, right-1 {
		buf[left], buf[right] = buf[right], buf[left]
	}
	return string(buf)
}

// 保证泛型 FixedMap 的具体类型仍与可信原版恢复出的类型一致。
var _ *maputils.FixedMap[string, *ccBlockedCounter] = ccBlockedCounterMap
