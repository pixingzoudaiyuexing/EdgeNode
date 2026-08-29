package cc

import (
	"container/list"
	"sync"
	"time"
)

type secondBucket struct {
	second int64
	count  uint32
}

type requestEntry struct {
	buckets []secondBucket
	lru     *list.Element
}

// RequestTracker 记录每个 key 的按秒请求数，并可查询最近任意 N 秒的请求总量。
//
// 当前阶段只提供“计数”能力，不在这里判断是否命中 CC 阈值。这样可以把尚未有
// 直接历史证据确认的 MaxRequests 边界（达到还是超过）和多阈值封禁选择规则留在
// 后续独立阶段处理，避免计数器本身夹带策略语义。
//
// 为防止随机源 IP 无限占用内存，Tracker 使用固定 key 容量和 LRU 淘汰；每个 key
// 仅保留 maxPeriodSeconds 个按秒桶。容量保护只影响极端资源耗尽场景，不改变正常
// key 在有效滚动窗口内的统计结果。
type RequestTracker struct {
	maxPeriodSeconds int
	capacity         int

	locker  sync.Mutex
	entries map[string]*requestEntry
	lru     *list.List
}

// NewRequestTracker 创建请求计数器。
// maxPeriodSeconds 必须大于 0；capacity 必须大于 0。
func NewRequestTracker(maxPeriodSeconds int, capacity int) *RequestTracker {
	if maxPeriodSeconds < 1 {
		maxPeriodSeconds = 1
	}
	if capacity < 1 {
		capacity = 1
	}
	return &RequestTracker{
		maxPeriodSeconds: maxPeriodSeconds,
		capacity:         capacity,
		entries:          make(map[string]*requestEntry, capacity),
		lru:              list.New(),
	}
}

// Record 为 key 在指定时间记录一次请求。
func (this *RequestTracker) Record(key string, now time.Time) {
	if key == "" {
		return
	}

	second := now.Unix()

	this.locker.Lock()
	defer this.locker.Unlock()

	entry := this.entries[key]
	if entry == nil {
		if len(this.entries) >= this.capacity {
			this.evictOldest()
		}
		entry = &requestEntry{
			buckets: make([]secondBucket, this.maxPeriodSeconds),
		}
		entry.lru = this.lru.PushBack(key)
		this.entries[key] = entry
	} else {
		this.lru.MoveToBack(entry.lru)
	}

	index := bucketIndex(second, this.maxPeriodSeconds)
	bucket := &entry.buckets[index]
	if bucket.second != second {
		bucket.second = second
		bucket.count = 0
	}
	bucket.count++
}

// Count 返回 key 在以 now 为当前秒、最近 periodSeconds 秒内的请求数。
// periodSeconds 超出 1..maxPeriodSeconds 时返回 0，调用方应根据实际策略窗口创建
// 足够大的 Tracker，而不是依赖静默截断。
func (this *RequestTracker) Count(key string, now time.Time, periodSeconds int) uint64 {
	if key == "" || periodSeconds < 1 || periodSeconds > this.maxPeriodSeconds {
		return 0
	}

	currentSecond := now.Unix()
	minSecond := currentSecond - int64(periodSeconds) + 1

	this.locker.Lock()
	defer this.locker.Unlock()

	entry := this.entries[key]
	if entry == nil {
		return 0
	}

	var total uint64
	for second := minSecond; second <= currentSecond; second++ {
		bucket := entry.buckets[bucketIndex(second, this.maxPeriodSeconds)]
		if bucket.second == second {
			total += uint64(bucket.count)
		}
	}
	return total
}

// Counts 一次返回多个时间窗的计数，避免调用方重复加锁。
// 非法窗口对应的结果为 0，返回顺序与 periods 相同。
func (this *RequestTracker) Counts(key string, now time.Time, periods []int) []uint64 {
	results := make([]uint64, len(periods))
	if key == "" || len(periods) == 0 {
		return results
	}

	currentSecond := now.Unix()

	this.locker.Lock()
	defer this.locker.Unlock()

	entry := this.entries[key]
	if entry == nil {
		return results
	}

	for i, periodSeconds := range periods {
		if periodSeconds < 1 || periodSeconds > this.maxPeriodSeconds {
			continue
		}
		minSecond := currentSecond - int64(periodSeconds) + 1
		var total uint64
		for second := minSecond; second <= currentSecond; second++ {
			bucket := entry.buckets[bucketIndex(second, this.maxPeriodSeconds)]
			if bucket.second == second {
				total += uint64(bucket.count)
			}
		}
		results[i] = total
	}
	return results
}

func (this *RequestTracker) evictOldest() {
	oldest := this.lru.Front()
	if oldest == nil {
		return
	}
	key, ok := oldest.Value.(string)
	if ok {
		delete(this.entries, key)
	}
	this.lru.Remove(oldest)
}

func bucketIndex(second int64, size int) int {
	index := second % int64(size)
	if index < 0 {
		index += int64(size)
	}
	return int(index)
}
