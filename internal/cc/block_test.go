package cc

import (
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/utils/fasttime"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
)

func TestRecordBlockedIPServerScope(t *testing.T) {
	list := waf.NewIPList(waf.IPListTypeDeny)
	const serverID int64 = 9_100_001
	const remoteAddr = "198.51.100.101"

	expiresAt := recordBlockedIP(list, serverID, remoteAddr, firewallconfigs.FirewallScopeServer, 60, false, "CC test")
	if expiresAt <= fasttime.Now().Unix() {
		t.Fatalf("过期时间无效: %d", expiresAt)
	}
	if !list.Contains(waf.IPTypeAll, firewallconfigs.FirewallScopeServer, serverID, remoteAddr) {
		t.Fatal("server scope 应写入对应服务临时黑名单")
	}
	if list.Contains(waf.IPTypeAll, firewallconfigs.FirewallScopeGlobal, 0, remoteAddr) {
		t.Fatal("server scope 不应同时写入 global 黑名单")
	}

	storedExpiresAt, ok := list.ContainsExpires(waf.IPTypeAll, firewallconfigs.FirewallScopeServer, serverID, remoteAddr)
	if !ok || storedExpiresAt != expiresAt {
		t.Fatalf("过期时间不一致: got=(%d,%v), want=%d", storedExpiresAt, ok, expiresAt)
	}
}

func TestRecordBlockedIPGlobalScope(t *testing.T) {
	list := waf.NewIPList(waf.IPListTypeDeny)
	const serverID int64 = 9_100_002
	const remoteAddr = "198.51.100.102"

	if expiresAt := recordBlockedIP(list, serverID, remoteAddr, firewallconfigs.FirewallScopeGlobal, 120, false, "CC test"); expiresAt <= 0 {
		t.Fatalf("应返回有效过期时间: %d", expiresAt)
	}
	if !list.Contains(waf.IPTypeAll, firewallconfigs.FirewallScopeGlobal, 0, remoteAddr) {
		t.Fatal("global scope 应写入全局临时黑名单")
	}
}

func TestRecordBlockedIPRejectsInvalidInputs(t *testing.T) {
	list := waf.NewIPList(waf.IPListTypeDeny)
	if expiresAt := recordBlockedIP(nil, 1, "198.51.100.103", firewallconfigs.FirewallScopeGlobal, 60, false, ""); expiresAt != 0 {
		t.Fatalf("nil list 应返回 0，实际 %d", expiresAt)
	}
	if expiresAt := recordBlockedIP(list, 0, "198.51.100.103", firewallconfigs.FirewallScopeGlobal, 60, false, ""); expiresAt != 0 {
		t.Fatalf("无效 serverId 应返回 0，实际 %d", expiresAt)
	}
	if expiresAt := recordBlockedIP(list, 1, "", firewallconfigs.FirewallScopeGlobal, 60, false, ""); expiresAt != 0 {
		t.Fatalf("空 IP 应返回 0，实际 %d", expiresAt)
	}
	if expiresAt := recordBlockedIP(list, 1, "198.51.100.103", firewallconfigs.FirewallScopeGlobal, 0, false, ""); expiresAt != 0 {
		t.Fatalf("无效 blockSeconds 应返回 0，实际 %d", expiresAt)
	}
}
