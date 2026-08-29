//go:build plus

package nodes

import (
	"net/http"
	"testing"
	"time"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/uam"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
)

func TestHTTPRequestIsUAMRequest(t *testing.T) {
	tests := []struct {
		name   string
		method string
		header string
		want   bool
	}{
		{name: "challenge post", method: http.MethodPost, header: uam.StepPrevious, want: true},
		{name: "header is case insensitive", method: http.MethodPost, header: "PREV", want: true},
		{name: "normal post", method: http.MethodPost, header: "", want: false},
		{name: "challenge header on get", method: http.MethodGet, header: uam.StepPrevious, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawReq, err := http.NewRequest(tt.method, "https://example.com/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.header != "" {
				rawReq.Header.Set(uam.StepHeader, tt.header)
			}

			req := &HTTPRequest{RawReq: rawReq}
			if got := req.isUAMRequest(); got != tt.want {
				t.Fatalf("isUAMRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPRequestRecordUAMChallengeFailure(t *testing.T) {
	const (
		serverID = int64(987654321)
		ip       = "203.0.113.88"
	)

	// 全局对象会被其它测试复用，因此使用固定测试专用 serverId/IP，并在前后清理。
	waf.SharedIPBlackList.RemoveIP(ip, serverID, false)
	defer waf.SharedIPBlackList.RemoveIP(ip, serverID, false)

	req := &HTTPRequest{
		ReqServer: &serverconfigs.ServerConfig{Id: serverID},
	}
	sharedUAMFailureTracker.Reset(req.uamFailureKey(ip))
	defer sharedUAMFailureTracker.Reset(req.uamFailureKey(ip))

	policy := nodeconfigs.NewUAMPolicy()
	policy.MaxFails = 2
	policy.BlockSeconds = 60
	policy.Firewall.Scope = firewallconfigs.FirewallScopeServer

	req.recordUAMChallengeFailure(ip, policy)
	if waf.SharedIPBlackList.Contains(waf.IPTypeAll, firewallconfigs.FirewallScopeServer, serverID, ip) {
		t.Fatal("未达到 MaxFails 前不应进入临时黑名单")
	}
	if count := sharedUAMFailureTracker.Count(req.uamFailureKey(ip)); count != 1 {
		t.Fatalf("第一次失败后计数应为 1，实际为 %d", count)
	}

	before := time.Now().Unix()
	req.recordUAMChallengeFailure(ip, policy)
	if !waf.SharedIPBlackList.Contains(waf.IPTypeAll, firewallconfigs.FirewallScopeServer, serverID, ip) {
		t.Fatal("达到 MaxFails 后应进入 server scope 临时黑名单")
	}
	if count := sharedUAMFailureTracker.Count(req.uamFailureKey(ip)); count != 0 {
		t.Fatalf("加入黑名单后连续失败计数应清零，实际为 %d", count)
	}

	expiresAt, ok := waf.SharedIPBlackList.ContainsExpires(waf.IPTypeAll, firewallconfigs.FirewallScopeServer, serverID, ip)
	if !ok {
		t.Fatal("应能读取 UAM 临时黑名单过期时间")
	}
	if expiresAt < before+int64(policy.BlockSeconds)-1 || expiresAt > time.Now().Unix()+int64(policy.BlockSeconds)+1 {
		t.Fatalf("黑名单过期时间未按 BlockSeconds 设置：%d", expiresAt)
	}
}
