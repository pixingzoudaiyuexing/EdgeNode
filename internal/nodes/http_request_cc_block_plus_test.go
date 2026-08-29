//go:build plus

package nodes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
)

func TestHTTPRequestDoCCBlocksOnNthThresholdRequest(t *testing.T) {
	const (
		serverID   int64 = 9_110_001
		clusterID  int64 = 7_101
		remoteAddr       = "198.51.100.101"
		period            = 60
		maxRequests       = 3
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()

	request, recorder := newCCThresholdBlockTestRequest(
		t,
		serverID,
		clusterID,
		remoteAddr,
		&serverconfigs.HTTPCCThreshold{
			PeriodSeconds: period,
			MaxRequests:   maxRequests,
			BlockSeconds:  0,
		},
	)
	qpsKey, thresholdKey := resetCCThresholdTestCounters(t, serverID, remoteAddr, request.RawReq.URL.Path, period)

	for i := 1; i < maxRequests; i++ {
		if request.doCC() {
			t.Fatalf("第 %d 次请求尚未达到阈值，不应阻断", i)
		}
	}
	if value := counters.SharedCounter.GetKey(thresholdKey); value != maxRequests-1 {
		t.Fatalf("N-1 次请求后的阈值计数应为 %d，实际 %d", maxRequests-1, value)
	}

	if !request.doCC() {
		t.Fatal("第 N 次请求达到 MaxRequests 时应立即阻断")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("阈值命中应返回 HTTP 429，实际 %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), ccTooManyRequestsEN) {
		t.Fatalf("429 页面应包含原版英文提示，实际 body=%q", recorder.Body.String())
	}
	if !request.isAttack {
		t.Fatal("CC 阈值命中后应标记为攻击请求")
	}
	if !containsCCRequestTag(request.tags, ccThresholdTag) {
		t.Fatalf("CC 阈值命中后应追加标签 %q，实际 %#v", ccThresholdTag, request.tags)
	}
	if !request.isDone {
		t.Fatal("CC 阈值命中后 Close() 应将请求标记为完成")
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != maxRequests {
		t.Fatalf("第 N 次请求后 QPS 计数应为 %d，实际 %d", maxRequests, value)
	}
	if value := counters.SharedCounter.GetKey(thresholdKey); value != maxRequests {
		t.Fatalf("第 N 次请求后阈值计数应为 %d，实际 %d", maxRequests, value)
	}
}

func TestHTTPRequestDoCCZeroBlockSecondsDoesNotBlacklistOrEscalate(t *testing.T) {
	const (
		serverID   int64 = 9_110_002
		clusterID  int64 = 7_102
		remoteAddr       = "198.51.100.102"
		period            = 17
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)
	defer waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)

	request, recorder := newCCThresholdBlockTestRequest(
		t,
		serverID,
		clusterID,
		remoteAddr,
		&serverconfigs.HTTPCCThreshold{PeriodSeconds: period, MaxRequests: 1, BlockSeconds: 0},
	)
	resetCCThresholdTestCounters(t, serverID, remoteAddr, request.RawReq.URL.Path, period)

	if !request.doCC() {
		t.Fatal("BlockSeconds=0 仍应在阈值命中时返回 429")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("BlockSeconds=0 时仍应返回 HTTP 429，实际 %d", recorder.Code)
	}
	if waf.SharedIPBlackList.Contains(waf.IPTypeAll, firewallconfigs.FirewallScopeServer, serverID, remoteAddr) {
		t.Fatal("BlockSeconds=0 不应写入临时黑名单")
	}
	if _, ok := ccBlockedCounterMap.Get(remoteAddr); ok {
		t.Fatal("BlockSeconds=0 不应增加重复封禁倍率状态")
	}
}

func TestHTTPRequestDoCCRecordsServerScopeBlacklist(t *testing.T) {
	const (
		serverID     int64 = 9_110_003
		clusterID    int64 = 7_103
		remoteAddr         = "198.51.100.103"
		period              = 23
		blockSeconds        = 120
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)
	defer waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)

	request, recorder := newCCThresholdBlockTestRequest(
		t,
		serverID,
		clusterID,
		remoteAddr,
		&serverconfigs.HTTPCCThreshold{PeriodSeconds: period, MaxRequests: 1, BlockSeconds: blockSeconds},
	)
	resetCCThresholdTestCounters(t, serverID, remoteAddr, request.RawReq.URL.Path, period)

	before := time.Now().Unix()
	if !request.doCC() {
		t.Fatal("达到阈值后应阻断并写入临时黑名单")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("达到阈值应返回 429，实际 %d", recorder.Code)
	}
	expiresAt, ok := waf.SharedIPBlackList.ContainsExpires(
		waf.IPTypeAll,
		firewallconfigs.FirewallScopeServer,
		serverID,
		remoteAddr,
	)
	if !ok {
		t.Fatal("server scope CC 封禁应写入临时黑名单")
	}
	if expiresAt < before+blockSeconds-2 || expiresAt > time.Now().Unix()+blockSeconds+2 {
		t.Fatalf("第一次封禁应约为 %d 秒，expiresAt=%d before=%d", blockSeconds, expiresAt, before)
	}
	counter, ok := ccBlockedCounterMap.Get(remoteAddr)
	if !ok || counter.count != 1 {
		t.Fatalf("第一次有时长封禁后倍率状态应为 1，实际 %#v ok=%v", counter, ok)
	}
}

func TestHTTPRequestDoCCEscalatesSameIPAcrossServers(t *testing.T) {
	const (
		serverID1    int64 = 9_110_004
		serverID2    int64 = 9_110_005
		clusterID1   int64 = 7_104
		clusterID2   int64 = 7_105
		remoteAddr         = "198.51.100.104"
		period              = 29
		blockSeconds        = 90
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID1, false)
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID2, false)
	defer waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID1, false)
	defer waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID2, false)

	threshold := &serverconfigs.HTTPCCThreshold{PeriodSeconds: period, MaxRequests: 1, BlockSeconds: blockSeconds}
	request1, _ := newCCThresholdBlockTestRequest(t, serverID1, clusterID1, remoteAddr, threshold)
	request2, _ := newCCThresholdBlockTestRequest(t, serverID2, clusterID2, remoteAddr, threshold)
	resetCCThresholdTestCounters(t, serverID1, remoteAddr, request1.RawReq.URL.Path, period)
	resetCCThresholdTestCounters(t, serverID2, remoteAddr, request2.RawReq.URL.Path, period)

	if !request1.doCC() {
		t.Fatal("第一个站点第一次封禁应触发")
	}
	if !request2.doCC() {
		t.Fatal("同 IP 在第二个站点再次触发时也应封禁")
	}

	counter, ok := ccBlockedCounterMap.Get(remoteAddr)
	if !ok || counter.count != 2 {
		t.Fatalf("同 IP 跨站点第二次触发后倍率应为 2，实际 %#v ok=%v", counter, ok)
	}
	expiresAt, ok := waf.SharedIPBlackList.ContainsExpires(
		waf.IPTypeAll,
		firewallconfigs.FirewallScopeServer,
		serverID2,
		remoteAddr,
	)
	if !ok {
		t.Fatal("第二个站点应存在 server scope 临时黑名单")
	}
	now := time.Now().Unix()
	if expiresAt < now+blockSeconds*2-3 || expiresAt > now+blockSeconds*2+3 {
		t.Fatalf("同 IP 第二次封禁应约为基础时长的 2 倍，expiresAt=%d now=%d", expiresAt, now)
	}
}

func TestHTTPRequestDoCCStopsAfterFirstMatchedThreshold(t *testing.T) {
	const (
		serverID   int64 = 9_110_006
		clusterID  int64 = 7_106
		remoteAddr       = "198.51.100.105"
		firstPeriod       = 31
		secondPeriod      = 37
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()

	req := httptest.NewRequest("GET", "https://example.com/first-threshold", nil)
	req.RemoteAddr = remoteAddr + ":12345"
	recorder := httptest.NewRecorder()
	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		Thresholds: []*serverconfigs.HTTPCCThreshold{
			{PeriodSeconds: firstPeriod, MaxRequests: 1, BlockSeconds: 0},
			{PeriodSeconds: secondPeriod, MaxRequests: 1, BlockSeconds: 0},
		},
	}
	policy := newServerScopeCCPolicy()
	nodeConfig := &nodeconfigs.NodeConfig{HTTPCCPolicies: map[int64]*nodeconfigs.HTTPCCPolicy{clusterID: policy}}
	request := &HTTPRequest{
		RawReq:     req,
		RawWriter:  recorder,
		ReqHost:    req.Host,
		ReqServer: &serverconfigs.ServerConfig{Id: serverID, ClusterId: clusterID},
		nodeConfig: nodeConfig,
		web:       &serverconfigs.HTTPWebConfig{CC: config},
		rawURI:    req.URL.RequestURI(),
		uri:       req.URL.RequestURI(),
	}
	request.writer = NewHTTPWriter(request, recorder)

	firstKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, req.URL.Path, false, firstPeriod)
	secondKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, req.URL.Path, false, secondPeriod)
	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	for _, key := range []string{firstKey, secondKey, qpsKey} {
		counters.SharedCounter.ResetKey(key)
		defer counters.SharedCounter.ResetKey(key)
	}

	if !request.doCC() {
		t.Fatal("第一条阈值命中后应立即阻断")
	}
	if got := counters.SharedCounter.GetKey(firstKey); got != 1 {
		t.Fatalf("第一条阈值应计数 1，实际 %d", got)
	}
	if got := counters.SharedCounter.GetKey(secondKey); got != 0 {
		t.Fatalf("第一条阈值命中后不应继续统计后续阈值，第二条实际 %d", got)
	}
}

func newCCThresholdBlockTestRequest(
	t *testing.T,
	serverID int64,
	clusterID int64,
	remoteAddr string,
	threshold *serverconfigs.HTTPCCThreshold,
) (*HTTPRequest, *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest("GET", "https://example.com/cc-threshold", nil)
	req.RemoteAddr = remoteAddr + ":12345"
	recorder := httptest.NewRecorder()
	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		Thresholds:           []*serverconfigs.HTTPCCThreshold{threshold.Clone()},
	}
	policy := newServerScopeCCPolicy()
	nodeConfig := &nodeconfigs.NodeConfig{
		HTTPCCPolicies: map[int64]*nodeconfigs.HTTPCCPolicy{clusterID: policy},
	}
	request := &HTTPRequest{
		RawReq:     req,
		RawWriter:  recorder,
		ReqHost:    req.Host,
		ReqServer: &serverconfigs.ServerConfig{Id: serverID, ClusterId: clusterID},
		nodeConfig: nodeConfig,
		web:       &serverconfigs.HTTPWebConfig{CC: config},
		rawURI:    req.URL.RequestURI(),
		uri:       req.URL.RequestURI(),
	}
	request.writer = NewHTTPWriter(request, recorder)
	return request, recorder
}

func newServerScopeCCPolicy() *nodeconfigs.HTTPCCPolicy {
	policy := nodeconfigs.NewHTTPCCPolicy()
	policy.Firewall.Scope = firewallconfigs.FirewallScopeServer
	return policy
}

func resetCCThresholdTestCounters(
	t *testing.T,
	serverID int64,
	remoteAddr string,
	requestPath string,
	period int,
) (qpsKey string, thresholdKey string) {
	t.Helper()
	qpsKey = edgecc.QPSCounterKey(serverID, remoteAddr)
	thresholdKey = edgecc.ThresholdCounterKey(serverID, remoteAddr, requestPath, false, period)
	counters.SharedCounter.ResetKey(qpsKey)
	counters.SharedCounter.ResetKey(thresholdKey)
	t.Cleanup(func() {
		counters.SharedCounter.ResetKey(qpsKey)
		counters.SharedCounter.ResetKey(thresholdKey)
	})
	return
}

func containsCCRequestTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}
