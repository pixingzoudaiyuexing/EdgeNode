package cc

import (
	"strings"
	"testing"

	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
)

func TestThresholdCounterKeySeparatesPeriodsAndPaths(t *testing.T) {
	base := ThresholdCounterKey(10, "1.2.3.4", "/a", false, 5)
	otherPeriod := ThresholdCounterKey(10, "1.2.3.4", "/a", false, 60)
	if base == otherPeriod {
		t.Fatal("不同统计周期必须使用不同 key")
	}

	withoutPathA := ThresholdCounterKey(10, "1.2.3.4", "/a", false, 5)
	withoutPathB := ThresholdCounterKey(10, "1.2.3.4", "/b", false, 5)
	if withoutPathA != withoutPathB {
		t.Fatal("WithRequestPath=false 时不应按路径拆分")
	}

	withPathA := ThresholdCounterKey(10, "1.2.3.4", "/a", true, 5)
	withPathB := ThresholdCounterKey(10, "1.2.3.4", "/b", true, 5)
	if withPathA == withPathB {
		t.Fatal("WithRequestPath=true 时必须按路径拆分")
	}
}

func TestQPSCounterKeyDoesNotContainPathDimension(t *testing.T) {
	key := QPSCounterKey(10, "1.2.3.4")
	if !strings.Contains(key, ":qps:10:1.2.3.4") {
		t.Fatalf("unexpected qps key: %q", key)
	}
}

func TestIncreaseThresholdUsesNativeCounter(t *testing.T) {
	serverID := int64(9_000_001)
	clientKey := "cc-test-threshold-client"
	period := 60
	key := ThresholdCounterKey(serverID, clientKey, "", false, period)
	counters.SharedCounter.ResetKey(key)
	defer counters.SharedCounter.ResetKey(key)

	if value := IncreaseThreshold(serverID, clientKey, "", false, period); value != 1 {
		t.Fatalf("第一次计数应为 1，实际 %d", value)
	}
	if value := IncreaseThreshold(serverID, clientKey, "", false, period); value != 2 {
		t.Fatalf("第二次计数应为 2，实际 %d", value)
	}
}

func TestIncreaseQPSUsesNativeCounter(t *testing.T) {
	serverID := int64(9_000_002)
	remoteAddr := "198.51.100.20"
	key := QPSCounterKey(serverID, remoteAddr)
	counters.SharedCounter.ResetKey(key)
	defer counters.SharedCounter.ResetKey(key)

	if value := IncreaseQPS(serverID, remoteAddr); value != 1 {
		t.Fatalf("第一次 QPS 计数应为 1，实际 %d", value)
	}
	if value := IncreaseQPS(serverID, remoteAddr); value != 2 {
		t.Fatalf("第二次 QPS 计数应为 2，实际 %d", value)
	}
}

func TestIncreaseCountersRejectInvalidInputs(t *testing.T) {
	if value := IncreaseThreshold(0, "1.2.3.4", "", false, 60); value != 0 {
		t.Fatalf("无效 serverId 应返回 0，实际 %d", value)
	}
	if value := IncreaseThreshold(1, "", "", false, 60); value != 0 {
		t.Fatalf("空 client key 应返回 0，实际 %d", value)
	}
	if value := IncreaseThreshold(1, "1.2.3.4", "", false, 0); value != 0 {
		t.Fatalf("无效 period 应返回 0，实际 %d", value)
	}
	if value := IncreaseQPS(0, "1.2.3.4"); value != 0 {
		t.Fatalf("无效 serverId 应返回 0，实际 %d", value)
	}
	if value := IncreaseQPS(1, ""); value != 0 {
		t.Fatalf("空 remote addr 应返回 0，实际 %d", value)
	}
}
