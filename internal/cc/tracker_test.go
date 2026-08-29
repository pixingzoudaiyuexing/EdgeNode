package cc

import (
	"sync"
	"testing"
	"time"
)

func TestRequestTrackerRollingWindows(t *testing.T) {
	tracker := NewRequestTracker(300, 100)
	base := time.Unix(1_700_000_000, 0)

	tracker.Record("ip-a", base)
	tracker.Record("ip-a", base)
	tracker.Record("ip-a", base.Add(4*time.Second))
	tracker.Record("ip-a", base.Add(9*time.Second))

	// 以 t=9s 为当前秒时，最近 5 个整秒桶是 5,6,7,8,9；t=4s 已离开窗口。
	if count := tracker.Count("ip-a", base.Add(9*time.Second), 5); count != 1 {
		t.Fatalf("最近 5 秒应有 1 次请求，实际 %d", count)
	}
	if count := tracker.Count("ip-a", base.Add(9*time.Second), 10); count != 4 {
		t.Fatalf("最近 10 秒应有 4 次请求，实际 %d", count)
	}
}

func TestRequestTrackerExpiresOldBuckets(t *testing.T) {
	tracker := NewRequestTracker(5, 100)
	base := time.Unix(1_700_000_000, 0)

	tracker.Record("ip-a", base)
	tracker.Record("ip-a", base.Add(5*time.Second))

	if count := tracker.Count("ip-a", base.Add(5*time.Second), 5); count != 1 {
		t.Fatalf("最早一秒已经离开 5 秒窗口，应只剩 1 次请求，实际 %d", count)
	}
}

func TestRequestTrackerRingReuseDoesNotLeakOldCount(t *testing.T) {
	tracker := NewRequestTracker(3, 100)
	base := time.Unix(1_700_000_000, 0)

	tracker.Record("ip-a", base)
	tracker.Record("ip-a", base.Add(3*time.Second))

	if count := tracker.Count("ip-a", base.Add(3*time.Second), 3); count != 1 {
		t.Fatalf("环形桶复用后不应残留旧计数，实际 %d", count)
	}
}

func TestRequestTrackerSeparatesKeys(t *testing.T) {
	tracker := NewRequestTracker(60, 100)
	now := time.Unix(1_700_000_000, 0)

	tracker.Record("ip-a", now)
	tracker.Record("ip-a", now)
	tracker.Record("ip-b", now)

	if count := tracker.Count("ip-a", now, 60); count != 2 {
		t.Fatalf("ip-a 应为 2，实际 %d", count)
	}
	if count := tracker.Count("ip-b", now, 60); count != 1 {
		t.Fatalf("ip-b 应为 1，实际 %d", count)
	}
}

func TestRequestTrackerCounts(t *testing.T) {
	tracker := NewRequestTracker(300, 100)
	base := time.Unix(1_700_000_000, 0)

	tracker.Record("ip-a", base)
	tracker.Record("ip-a", base.Add(4*time.Second))
	tracker.Record("ip-a", base.Add(59*time.Second))

	counts := tracker.Counts("ip-a", base.Add(59*time.Second), []int{5, 60, 300, 301, 0})
	expected := []uint64{1, 3, 3, 0, 0}
	for i := range expected {
		if counts[i] != expected[i] {
			t.Fatalf("窗口 %d 期望 %d，实际 %d", i, expected[i], counts[i])
		}
	}
}

func TestRequestTrackerLRUEviction(t *testing.T) {
	tracker := NewRequestTracker(60, 2)
	now := time.Unix(1_700_000_000, 0)

	tracker.Record("ip-a", now)
	tracker.Record("ip-b", now)
	// 再访问 a，使 b 成为最旧 key。
	tracker.Record("ip-a", now.Add(time.Second))
	tracker.Record("ip-c", now.Add(2*time.Second))

	if count := tracker.Count("ip-b", now.Add(2*time.Second), 60); count != 0 {
		t.Fatalf("最旧的 ip-b 应被淘汰，实际仍有 %d", count)
	}
	if count := tracker.Count("ip-a", now.Add(2*time.Second), 60); count != 2 {
		t.Fatalf("ip-a 应保留，实际 %d", count)
	}
	if count := tracker.Count("ip-c", now.Add(2*time.Second), 60); count != 1 {
		t.Fatalf("ip-c 应保留，实际 %d", count)
	}
}

func TestRequestTrackerInvalidInputs(t *testing.T) {
	tracker := NewRequestTracker(0, 0)
	now := time.Unix(1_700_000_000, 0)

	tracker.Record("", now)
	tracker.Record("ip-a", now)

	if count := tracker.Count("", now, 1); count != 0 {
		t.Fatalf("空 key 应返回 0，实际 %d", count)
	}
	if count := tracker.Count("ip-a", now, 0); count != 0 {
		t.Fatalf("非法窗口应返回 0，实际 %d", count)
	}
	if count := tracker.Count("ip-a", now, 2); count != 0 {
		t.Fatalf("超过最大窗口应返回 0，实际 %d", count)
	}
	if count := tracker.Count("ip-a", now, 1); count != 1 {
		t.Fatalf("构造参数下限应自动修正，实际 %d", count)
	}
}

func TestRequestTrackerConcurrentRecord(t *testing.T) {
	tracker := NewRequestTracker(60, 100)
	now := time.Unix(1_700_000_000, 0)

	const workers = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			tracker.Record("ip-a", now)
		}()
	}
	wg.Wait()

	if count := tracker.Count("ip-a", now, 60); count != workers {
		t.Fatalf("并发记录应为 %d，实际 %d", workers, count)
	}
}

func TestBucketIndexSupportsNegativeUnixTime(t *testing.T) {
	if index := bucketIndex(-1, 5); index != 4 {
		t.Fatalf("负时间索引应归一化为 4，实际 %d", index)
	}
}
