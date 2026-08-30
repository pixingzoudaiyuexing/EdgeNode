//go:build plus

package nodes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/utils/agents"
	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
	"github.com/TeaOSLab/EdgeNode/internal/utils/fasttime"
	"github.com/TeaOSLab/EdgeNode/internal/utils/ttlcache"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
)

func TestCCGET302KeyMatches139Protocol(t *testing.T) {
	const expected = "233c2ee3bcbd9b386f7d0887f15a0788.c7c9ba2652cdda7be2b561fc9d2eea22.1725000000"
	actual := ccGET302Key(
		"node-secret",
		"https://example.com/path?a=1",
		"198.51.100.200",
		1725000000,
	)
	if actual != expected {
		t.Fatalf("GET302 key 与 1.3.9 协议不一致：\nwant %s\n got %s", expected, actual)
	}
}

func TestHTTPRequestCCGET302IssuesOriginalChallenge(t *testing.T) {
	const (
		serverID   int64 = 9_130_001
		remoteAddr       = "198.51.100.201"
		secret           = "get302-secret-1"
	)
	request, recorder, config, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, "https://example.com/path?a=1", remoteAddr, secret)
	resetCCGET302TestState(t, serverID, remoteAddr)

	if !request.doCCGET302(config, policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("未获得临时权限的 GET 请求应进入 GET302 challenge")
	}
	if recorder.Code != http.StatusFound {
		t.Fatalf("GET302 challenge 应返回 302，实际 %d", recorder.Code)
	}

	location := recorder.Header().Get("Location")
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	if redirectURL.Path != nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingValidatePath {
		t.Fatalf("validator path 不一致，实际 %q", redirectURL.Path)
	}

	fullURL := "https://example.com/path?a=1"
	if got := redirectURL.Query().Get("url"); got != fullURL {
		t.Fatalf("GET302 url 参数不一致，want %q got %q", fullURL, got)
	}
	parts := strings.Split(redirectURL.Query().Get("key"), ".")
	if len(parts) != 3 {
		t.Fatalf("GET302 key 应为三段式，实际 %q", redirectURL.Query().Get("key"))
	}
	timestamp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		t.Fatalf("GET302 timestamp 无效：%v", err)
	}
	if got := ccGET302Key(secret, fullURL, remoteAddr, timestamp); got != redirectURL.Query().Get("key") {
		t.Fatalf("challenge key 校验失败，want %q got %q", got, redirectURL.Query().Get("key"))
	}
	if delta := fasttime.Now().Unix() - timestamp; delta < 0 || delta > 2 {
		t.Fatalf("challenge timestamp 应使用当前节点 Unix 秒，delta=%d", delta)
	}
	if got := counters.SharedCounter.GetKey(ccGET302RedirectCounterKey(remoteAddr)); got != 1 {
		t.Fatalf("发起 challenge 前应增加一次连续跳转计数，实际 %d", got)
	}
}

func TestHTTPRequestCCGET302FreshCallbackGrantsPermission(t *testing.T) {
	const (
		serverID   int64 = 9_130_002
		remoteAddr       = "198.51.100.202"
		secret           = "get302-secret-2"
	)
	originalURL := "https://example.com/account?from=test"
	timestamp := fasttime.Now().Unix()
	key := ccGET302Key(secret, originalURL, remoteAddr, timestamp)
	callbackURL := "https://example.com" + nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingValidatePath +
		"?key=" + url.QueryEscape(key) + "&url=" + url.QueryEscape(originalURL)

	request, recorder, config, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, callbackURL, remoteAddr, secret)
	resetCCGET302TestState(t, serverID, remoteAddr)

	if !request.doCCGET302(config, policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("合法 validator callback 应由 GET302 分支处理")
	}
	if recorder.Code != http.StatusFound {
		t.Fatalf("合法 callback 应 302 返回原 URL，实际 %d", recorder.Code)
	}
	if got := recorder.Header().Get("Location"); got != originalURL {
		t.Fatalf("合法 callback 回跳地址不一致，want %q got %q", originalURL, got)
	}
	item := ttlcache.SharedInt64Cache.Read(ccGET302PermissionKey(remoteAddr))
	if item == nil || item.Value != 1 {
		t.Fatalf("合法 callback 应写入 600 秒临时权限，实际 %#v", item)
	}
}

func TestHTTPRequestCCGET302RejectsInvalidAndRecentReplay(t *testing.T) {
	const (
		serverID   int64 = 9_130_003
		remoteAddr       = "198.51.100.203"
		secret           = "get302-secret-3"
	)
	originalURL := "https://example.com/replay"

	t.Run("invalid hash", func(t *testing.T) {
		callbackURL := "https://example.com" + nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingValidatePath +
			"?key=bad.bad.123&url=" + url.QueryEscape(originalURL)
		request, recorder, config, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, callbackURL, remoteAddr, secret)
		resetCCGET302TestState(t, serverID, remoteAddr)
		if !request.doCCGET302(config, policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
			t.Fatal("非法 callback 应被处理")
		}
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("非法 callback 应返回 403，实际 %d", recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), ccConnections403EN) {
			t.Fatalf("403 页面应包含原版提示，实际 %q", recorder.Body.String())
		}
	})

	t.Run("20 second replay", func(t *testing.T) {
		timestamp := fasttime.Now().Unix() - 20
		key := ccGET302Key(secret, originalURL, remoteAddr, timestamp)
		callbackURL := "https://example.com" + nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingValidatePath +
			"?key=" + url.QueryEscape(key) + "&url=" + url.QueryEscape(originalURL)
		request, recorder, config, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, callbackURL, remoteAddr, secret)
		resetCCGET302TestState(t, serverID, remoteAddr)
		if !request.doCCGET302(config, policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
			t.Fatal("10~300 秒 replay 应由 GET302 分支处理")
		}
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("10~300 秒 replay 应返回 403，实际 %d", recorder.Code)
		}
		if ttlcache.SharedInt64Cache.Read(ccGET302PermissionKey(remoteAddr)) != nil {
			t.Fatal("replay 不应获得临时权限")
		}
	})
}

func TestHTTPRequestCCGET302VeryStaleCallbackRedirectsWithoutPermission(t *testing.T) {
	const (
		serverID   int64 = 9_130_004
		remoteAddr       = "198.51.100.204"
		secret           = "get302-secret-4"
	)
	originalURL := "https://example.com/stale"
	timestamp := fasttime.Now().Unix() - 400
	key := ccGET302Key(secret, originalURL, remoteAddr, timestamp)
	callbackURL := "https://example.com" + nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingValidatePath +
		"?key=" + url.QueryEscape(key) + "&url=" + url.QueryEscape(originalURL)
	request, recorder, config, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, callbackURL, remoteAddr, secret)
	resetCCGET302TestState(t, serverID, remoteAddr)

	if !request.doCCGET302(config, policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("超过 300 秒的合法旧 callback 应被处理")
	}
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != originalURL {
		t.Fatalf("旧 callback 应只回跳原 URL，code=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
	if ttlcache.SharedInt64Cache.Read(ccGET302PermissionKey(remoteAddr)) != nil {
		t.Fatal("超过 300 秒的 callback 不应获得临时权限")
	}
}

func TestHTTPRequestCCGET302PermissionContinuesToThresholds(t *testing.T) {
	const (
		serverID   int64 = 9_130_005
		clusterID  int64 = 7_305
		remoteAddr       = "198.51.100.205"
		secret           = "get302-secret-5"
		period            = 60
	)
	request, recorder, config, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, "https://example.com/threshold", remoteAddr, secret)
	resetCCGET302TestState(t, serverID, remoteAddr)

	config.UseDefaultThresholds = false
	config.Thresholds = []*serverconfigs.HTTPCCThreshold{{PeriodSeconds: period, MaxRequests: 1, BlockSeconds: 0}}
	policy.MaxConnectionsPerIP = 1000
	request.ReqServer.ClusterId = clusterID
	request.nodeConfig.HTTPCCPolicies = map[int64]*nodeconfigs.HTTPCCPolicy{clusterID: policy}

	now := fasttime.Now().Unix()
	ttlcache.SharedInt64Cache.Write(ccGET302PermissionKey(remoteAddr), 1, now+ccGET302PermissionLife)

	if !request.doCC() {
		t.Fatal("已有 GET302 临时权限时应继续进入普通 CC 阈值，而不是直接放行整个 CC")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("临时权限后普通阈值仍应生效，实际 HTTP %d", recorder.Code)
	}
}

func TestHTTPRequestCheckCCRedirectsBlocksAtConfiguredLimit(t *testing.T) {
	const (
		serverID   int64 = 9_130_006
		remoteAddr       = "198.51.100.206"
	)
	request, _, _, policy := newCCGET302TestRequest(t, serverID, http.MethodGet, "https://example.com/loop", remoteAddr, "secret")
	resetCCGET302TestState(t, serverID, remoteAddr)
	policy.RedirectsChecking.DurationSeconds = 120
	policy.RedirectsChecking.MaxRedirects = 3
	policy.RedirectsChecking.BlockSeconds = 3600
	policy.Firewall.Scope = firewallconfigs.FirewallScopeServer

	if !request.checkCCRedirects(policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("第 1 次跳转不应封禁")
	}
	if !request.checkCCRedirects(policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("第 2 次跳转不应封禁")
	}
	before := time.Now().Unix()
	if request.checkCCRedirects(policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("第 3 次跳转在 count == MaxRedirects 时应立即封禁")
	}

	expiresAt, ok := waf.SharedIPBlackList.ContainsExpires(
		waf.IPTypeAll,
		firewallconfigs.FirewallScopeServer,
		serverID,
		remoteAddr,
	)
	if !ok {
		t.Fatal("连续无效跳转应写入 server scope 临时黑名单")
	}
	if expiresAt < before+3598 || expiresAt > time.Now().Unix()+3602 {
		t.Fatalf("连续跳转封禁应使用固定 3600 秒，不走 1~32 倍倍率，expiresAt=%d", expiresAt)
	}
	if _, ok := ccBlockedCounterMap.Get(remoteAddr); ok {
		t.Fatal("GET302 连续跳转封禁不应修改普通 CC 重复封禁倍率")
	}
}

func TestCCGET302SkipRules(t *testing.T) {
	if !ccGET302IsSearchProvider("百度搜索") || !ccGET302IsSearchProvider("谷歌线路") ||
		!ccGET302IsSearchProvider("baidu spider") || !ccGET302IsSearchProvider("google crawler") {
		t.Fatal("原版四类搜索引擎 Provider 关键字应全部识别")
	}
	if ccGET302IsSearchProvider("Baidu") {
		t.Fatal("原版使用大小写敏感的英文关键字，不应自行扩展成大小写不敏感匹配")
	}

	const remoteAddr = "198.51.100.207"
	request, recorder, config, policy := newCCGET302TestRequest(t, 9_130_007, http.MethodPost, "https://example.com/form", remoteAddr, "secret")
	resetCCGET302TestState(t, 9_130_007, remoteAddr)
	if request.doCCGET302(config, policy, remoteAddr, firewallconfigs.FirewallScopeServer) {
		t.Fatal("非 GET 请求不应进入 GET302")
	}
	if recorder.Header().Get("Location") != "" {
		t.Fatal("非 GET 请求不应产生重定向")
	}

	request, _, config, policy = newCCGET302TestRequest(t, 9_130_008, http.MethodGet, "https://example.com/baidu_verify_test.html", "198.51.100.208", "secret")
	if request.doCCGET302(config, policy, "198.51.100.208", firewallconfigs.FirewallScopeServer) {
		t.Fatal("/baidu_verify_ 前缀应跳过 GET302")
	}

	agentIP := "198.51.100.209"
	agents.SharedManager.AddIP(agentIP, "cc-get302-test")
	request, _, config, policy = newCCGET302TestRequest(t, 9_130_009, http.MethodGet, "https://example.com/normal", agentIP, "secret")
	if request.doCCGET302(config, policy, agentIP, firewallconfigs.FirewallScopeServer) {
		t.Fatal("已识别的客户端 Agent IP 应跳过 GET302")
	}
}

func newCCGET302TestRequest(t *testing.T, serverID int64, method string, rawURL string, remoteAddr string, secret string) (*HTTPRequest, *httptest.ResponseRecorder, *serverconfigs.HTTPCCConfig, *nodeconfigs.HTTPCCPolicy) {
	t.Helper()

	req := httptest.NewRequest(method, rawURL, nil)
	req.RemoteAddr = remoteAddr + ":12345"
	recorder := httptest.NewRecorder()
	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		EnableGET302:         true,
		UseDefaultThresholds: false,
	}
	policy := nodeconfigs.NewHTTPCCPolicy()
	policy.MaxConnectionsPerIP = 1000
	policy.Firewall.Scope = firewallconfigs.FirewallScopeServer
	nodeConfig := &nodeconfigs.NodeConfig{Secret: secret}
	requestURI := req.URL.RequestURI()
	request := &HTTPRequest{
		RawReq:     req,
		RawWriter:  recorder,
		ReqHost:    req.Host,
		ReqServer: &serverconfigs.ServerConfig{Id: serverID},
		IsHTTPS:    req.URL.Scheme == "https",
		nodeConfig: nodeConfig,
		web:        &serverconfigs.HTTPWebConfig{CC: config},
		rawURI:     requestURI,
		uri:        requestURI,
	}
	request.writer = NewHTTPWriter(request, recorder)

	oldSharedNodeConfig := sharedNodeConfig
	sharedNodeConfig = nodeConfig
	t.Cleanup(func() { sharedNodeConfig = oldSharedNodeConfig })
	return request, recorder, config, policy
}

func resetCCGET302TestState(t *testing.T, serverID int64, remoteAddr string) {
	t.Helper()
	permissionKey := ccGET302PermissionKey(remoteAddr)
	redirectKey := ccGET302RedirectCounterKey(remoteAddr)
	ttlcache.SharedInt64Cache.Delete(permissionKey)
	counters.SharedCounter.ResetKey(redirectKey)
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)
	t.Cleanup(func() {
		ttlcache.SharedInt64Cache.Delete(permissionKey)
		counters.SharedCounter.ResetKey(redirectKey)
		waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)
	})
}
