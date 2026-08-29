package cc

import (
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
)

func TestResolveMaxConnectionsPerIP(t *testing.T) {
	if got := ResolveMaxConnectionsPerIP(nil); got != DefaultMaxConnectionsPerIP {
		t.Fatalf("nil policy 应回退到 %d，实际 %d", DefaultMaxConnectionsPerIP, got)
	}

	policy := &nodeconfigs.HTTPCCPolicy{IsOn: true, MaxConnectionsPerIP: 55}
	if got := ResolveMaxConnectionsPerIP(policy); got != 55 {
		t.Fatalf("显式 MaxConnectionsPerIP 未生效，实际 %d", got)
	}

	policy.MaxConnectionsPerIP = 0
	if got := ResolveMaxConnectionsPerIP(policy); got != DefaultMaxConnectionsPerIP {
		t.Fatalf("非正数应回退到 %d，实际 %d", DefaultMaxConnectionsPerIP, got)
	}
}

func TestMaxConnectionsReachedAtEquality(t *testing.T) {
	if MaxConnectionsReached(29, 30) {
		t.Fatal("29/30 不应触发")
	}
	if !MaxConnectionsReached(30, 30) {
		t.Fatal("30/30 应在等于阈值时立即触发")
	}
	if !MaxConnectionsReached(31, 30) {
		t.Fatal("超过阈值应触发")
	}
	if MaxConnectionsReached(1, 0) {
		t.Fatal("无效连接上限不应触发")
	}
}

func TestMaxConnectionsBlockSeconds(t *testing.T) {
	if MaxConnectionsBlockSeconds != 1800 {
		t.Fatalf("原版基础封禁时长应为 1800 秒，实际 %d", MaxConnectionsBlockSeconds)
	}
}
