package uam

import (
	"sync"
	"time"
)

const qpsWindowSeconds int64 = 60

// QPSTracker 用一个 60 秒滚动窗口统计单个键的请求量。
// GoEdge 文档把 MinQPSPerIP 定义为“1 分钟内平均 QPS”，因此触发条件为
// 最近 60 秒累计请求数 >= MinQPSPerIP * 60。
type QPSTracker struct {
	locker  sync.Mutex
	entries map[string]*qpsEntry
	ops     uint64
}

type qpsEntry struct {
	buckets    [qpsWindowSeconds]int64
	total      int64
	lastSecond int64
	lastSeen   int64
}

func NewQPSTracker() *QPSTracker {
	return &QPSTracker{entries: map[string]*qpsEntry{}}
}

// Triggered 记录一次请求并返回是否达到最低 QPS。
// minQPS <= 0 表示所有请求都应进入 UAM，这与管理端公开语义保持一致。
func (t *QPSTracker) Triggered(key string, minQPS int) bool {
	return t.triggeredAt(key, minQPS, time.Now().Unix())
}

func (t *QPSTracker) triggeredAt(key string, minQPS int, now int64) bool {
	if minQPS <= 0 {
		return true
	}
	if key == "" {
		return false
	}

	t.locker.Lock()
	defer t.locker.Unlock()

	if t.entries == nil {
		t.entries = map[string]*qpsEntry{}
	}
	entry := t.entries[key]
	if entry == nil {
		entry = &qpsEntry{lastSecond: now, lastSeen: now}
		t.entries[key] = entry
	} else {
		t.advance(entry, now)
	}

	index := positiveModulo(now, qpsWindowSeconds)
	entry.buckets[index]++
	entry.total++
	entry.lastSecond = now
	entry.lastSeen = now

	t.ops++
	if t.ops%1024 == 0 {
		t.cleanup(now)
	}

	threshold := int64(minQPS) * qpsWindowSeconds
	return threshold > 0 && entry.total >= threshold
}

func (t *QPSTracker) advance(entry *qpsEntry, now int64) {
	if entry == nil {
		return
	}

	delta := now - entry.lastSecond
	if delta <= 0 {
		return
	}
	if delta >= qpsWindowSeconds {
		entry.buckets = [qpsWindowSeconds]int64{}
		entry.total = 0
		entry.lastSecond = now
		return
	}

	for second := entry.lastSecond + 1; second <= now; second++ {
		index := positiveModulo(second, qpsWindowSeconds)
		entry.total -= entry.buckets[index]
		entry.buckets[index] = 0
	}
	entry.lastSecond = now
}

func (t *QPSTracker) cleanup(now int64) {
	for key, entry := range t.entries {
		if entry == nil || now-entry.lastSeen > 2*qpsWindowSeconds {
			delete(t.entries, key)
		}
	}
}

func positiveModulo(value int64, divisor int64) int {
	result := value % divisor
	if result < 0 {
		result += divisor
	}
	return int(result)
}
