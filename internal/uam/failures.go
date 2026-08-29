package uam

import (
	"sync"
	"time"
)

const defaultMaxFailureEntries = 100_000

type failureEntry struct {
	count     int
	updatedAt int64
}

// FailureTracker 记录同一访问者连续未通过 UAM Challenge 的次数。
//
// 1.3.x 的公开行为只明确“连续 N 次验证失败”会触发封禁，没有证据表明
// 计数会按固定时间窗口自动清零，因此这里不额外引入未经确认的时间窗口；
// 挑战成功时由调用方显式 Reset。
//
// maxEntries 只用于防止大量随机 IP/Key 长期占用内存。达到上限时淘汰最久未更新项，
// 这是资源保护边界，不参与正常访问者的失败次数判定。
type FailureTracker struct {
	locker     sync.Mutex
	entries    map[string]failureEntry
	maxEntries int
}

func NewFailureTracker() *FailureTracker {
	return newFailureTracker(defaultMaxFailureEntries)
}

func newFailureTracker(maxEntries int) *FailureTracker {
	if maxEntries <= 0 {
		maxEntries = defaultMaxFailureEntries
	}
	return &FailureTracker{
		entries:    map[string]failureEntry{},
		maxEntries: maxEntries,
	}
}

// Increase 增加一次连续失败并返回当前次数。
func (t *FailureTracker) Increase(key string) int {
	if t == nil || key == "" {
		return 0
	}

	t.locker.Lock()
	defer t.locker.Unlock()

	entry, exists := t.entries[key]
	if !exists && len(t.entries) >= t.maxEntries {
		t.removeOldestLocked()
	}

	entry.count++
	entry.updatedAt = time.Now().UnixNano()
	t.entries[key] = entry
	return entry.count
}

// Reset 在挑战成功后清除连续失败次数。
func (t *FailureTracker) Reset(key string) {
	if t == nil || key == "" {
		return
	}

	t.locker.Lock()
	delete(t.entries, key)
	t.locker.Unlock()
}

// Count 返回当前连续失败次数，仅用于测试和诊断。
func (t *FailureTracker) Count(key string) int {
	if t == nil || key == "" {
		return 0
	}

	t.locker.Lock()
	entry := t.entries[key]
	t.locker.Unlock()
	return entry.count
}

func (t *FailureTracker) removeOldestLocked() {
	var oldestKey string
	var oldestUpdatedAt int64
	for key, entry := range t.entries {
		if oldestKey == "" || entry.updatedAt < oldestUpdatedAt {
			oldestKey = key
			oldestUpdatedAt = entry.updatedAt
		}
	}
	if oldestKey != "" {
		delete(t.entries, oldestKey)
	}
}
