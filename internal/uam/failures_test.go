package uam

import "testing"

func TestFailureTrackerIncreaseAndReset(t *testing.T) {
	tracker := newFailureTracker(10)

	if count := tracker.Increase("1@example"); count != 1 {
		t.Fatalf("第一次失败应为 1，实际为 %d", count)
	}
	if count := tracker.Increase("1@example"); count != 2 {
		t.Fatalf("第二次连续失败应为 2，实际为 %d", count)
	}
	if count := tracker.Count("1@example"); count != 2 {
		t.Fatalf("当前失败次数应为 2，实际为 %d", count)
	}

	tracker.Reset("1@example")
	if count := tracker.Count("1@example"); count != 0 {
		t.Fatalf("挑战成功后失败次数应清零，实际为 %d", count)
	}
	if count := tracker.Increase("1@example"); count != 1 {
		t.Fatalf("清零后的下一次失败应重新从 1 开始，实际为 %d", count)
	}
}

func TestFailureTrackerCapacityEvictsOldest(t *testing.T) {
	tracker := newFailureTracker(2)
	tracker.Increase("oldest")
	tracker.Increase("newer")

	// 避免依赖相同时间戳下 map 遍历顺序，直接固定测试时间顺序。
	tracker.locker.Lock()
	oldest := tracker.entries["oldest"]
	oldest.updatedAt = 1
	tracker.entries["oldest"] = oldest
	newer := tracker.entries["newer"]
	newer.updatedAt = 2
	tracker.entries["newer"] = newer
	tracker.locker.Unlock()

	tracker.Increase("latest")

	if count := tracker.Count("oldest"); count != 0 {
		t.Fatalf("达到容量上限后应淘汰最旧项，实际仍为 %d", count)
	}
	if count := tracker.Count("newer"); count != 1 {
		t.Fatalf("较新的失败计数不应被淘汰，实际为 %d", count)
	}
	if count := tracker.Count("latest"); count != 1 {
		t.Fatalf("新失败项应被记录，实际为 %d", count)
	}
}

func TestFailureTrackerEmptyKey(t *testing.T) {
	tracker := NewFailureTracker()
	if count := tracker.Increase(""); count != 0 {
		t.Fatalf("空 key 不应记录失败次数，实际为 %d", count)
	}
	tracker.Reset("")
	if count := tracker.Count(""); count != 0 {
		t.Fatalf("空 key 计数应始终为 0，实际为 %d", count)
	}
}
