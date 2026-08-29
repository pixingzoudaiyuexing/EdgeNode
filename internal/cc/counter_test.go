package cc

import (
	"encoding/hex"
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

func TestRedirectCounterKeySeparatesDurations(t *testing.T) {
	shortWindow := RedirectCounterKey(10, "1.2.3.4", 60)
	longWindow := RedirectCounterKey(10, "1.2.3.4", 120)
	if shortWindow == longWindow {
		t.Fatal("不同连续重定向统计周期必须使用不同 key")
	}
	if !strings.Contains(longWindow, ":redirect:10:120:1.2.3.4") {
		t.Fatalf("unexpected redirect key: %q", longWindow)
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

func TestIncreaseThresholdWithFingerprintUsesLargerValue(t *testing.T) {
	serverID := int64(9_000_003)
	remoteAddr := "198.51.100.30"
	fingerprint := []byte{0x01, 0x02, 0xab, 0xcd}
	period := 60
	ipKey := ThresholdCounterKey(serverID, remoteAddr, "", false, period)
	fpClientKey := hex.EncodeToString(fingerprint)
	fpKey := ThresholdCounterKey(serverID, fpClientKey, "", false, period)
	counters.SharedCounter.ResetKey(ipKey)
	counters.SharedCounter.ResetKey(fpKey)
	defer counters.SharedCounter.ResetKey(ipKey)
	defer counters.SharedCounter.ResetKey(fpKey)

	// 先给同一指纹制造更高的历史计数，再模拟当前请求同时增加 IP 和指纹。
	IncreaseThreshold(serverID, fpClientKey, "", false, period)
	IncreaseThreshold(serverID, fpClientKey, "", false, period)

	value := IncreaseThresholdWithFingerprint(serverID, remoteAddr, "", false, period, true, fingerprint)
	if value != 3 {
		t.Fatalf("应返回较大的指纹计数 3，实际 %d", value)
	}
	if ipValue := counters.SharedCounter.GetKey(ipKey); ipValue != 1 {
		t.Fatalf("IP 计数应独立为 1，实际 %d", ipValue)
	}
}

func TestIncreaseThresholdWithFingerprintCanBeDisabled(t *testing.T) {
	serverID := int64(9_000_004)
	remoteAddr := "198.51.100.40"
	fingerprint := []byte{0xaa, 0xbb}
	period := 60
	ipKey := ThresholdCounterKey(serverID, remoteAddr, "", false, period)
	fpKey := ThresholdCounterKey(serverID, hex.EncodeToString(fingerprint), "", false, period)
	counters.SharedCounter.ResetKey(ipKey)
	counters.SharedCounter.ResetKey(fpKey)
	defer counters.SharedCounter.ResetKey(ipKey)
	defer counters.SharedCounter.ResetKey(fpKey)

	if value := IncreaseThresholdWithFingerprint(serverID, remoteAddr, "", false, period, false, fingerprint); value != 1 {
		t.Fatalf("关闭指纹后应只返回 IP 计数 1，实际 %d", value)
	}
	if fpValue := counters.SharedCounter.GetKey(fpKey); fpValue != 0 {
		t.Fatalf("关闭指纹后不应增加指纹计数，实际 %d", fpValue)
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

func TestIncreaseRedirectUsesNativeCounter(t *testing.T) {
	serverID := int64(9_000_005)
	remoteAddr := "198.51.100.50"
	duration := 120
	key := RedirectCounterKey(serverID, remoteAddr, duration)
	counters.SharedCounter.ResetKey(key)
	defer counters.SharedCounter.ResetKey(key)

	if value := IncreaseRedirect(serverID, remoteAddr, duration); value != 1 {
		t.Fatalf("第一次连续重定向计数应为 1，实际 %d", value)
	}
	if value := IncreaseRedirect(serverID, remoteAddr, duration); value != 2 {
		t.Fatalf("第二次连续重定向计数应为 2，实际 %d", value)
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
	if value := IncreaseRedirect(0, "1.2.3.4", 120); value != 0 {
		t.Fatalf("无效 redirect serverId 应返回 0，实际 %d", value)
	}
	if value := IncreaseRedirect(1, "", 120); value != 0 {
		t.Fatalf("空 redirect remote addr 应返回 0，实际 %d", value)
	}
	if value := IncreaseRedirect(1, "1.2.3.4", 0); value != 0 {
		t.Fatalf("无效 redirect duration 应返回 0，实际 %d", value)
	}
}
