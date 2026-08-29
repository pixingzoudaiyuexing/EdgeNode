//go:build plus

package nodes

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/conns"
	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
)

func TestHTTPRequestDoCCBlocksAtMaxConnectionsWithEmptyThresholds(t *testing.T) {
	const (
		serverID   int64 = 9_120_001
		clusterID  int64 = 7_201
		remoteAddr       = "198.51.100.121"
		limit             = 2
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)
	defer waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)

	request, recorder := newCCMaxConnectionsTestRequest(t, serverID, clusterID, remoteAddr, limit, 0)
	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(qpsKey)

	first := addCCMapTestConn(t, remoteAddr, 22001)
	if request.doCC() {
		t.Fatal("N-1 条连接时不应触发 MaxConnectionsPerIP")
	}
	if first.closed {
		t.Fatal("未达到连接上限时不应关闭现有连接")
	}

	second := addCCMapTestConn(t, remoteAddr, 22002)
	before := time.Now().Unix()
	if !request.doCC() {
		t.Fatal("连接数等于 MaxConnectionsPerIP 时应立即阻断")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("连接上限命中应返回 HTTP 403，实际 %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), ccConnections403EN) {
		t.Fatalf("403 页面应包含原版英文提示，实际 body=%q", recorder.Body.String())
	}
	if request.isAttack {
		t.Fatal("原版连接限制分支不应设置 isAttack")
	}
	if containsCCRequestTag(request.tags, ccThresholdTag) {
		t.Fatalf("原版连接限制分支不应追加 %q 标签", ccThresholdTag)
	}
	if !first.closed || !second.closed {
		t.Fatal("连接限制写入临时黑名单后应关闭该 IP 的已登记连接")
	}

	expiresAt, ok := waf.SharedIPBlackList.ContainsExpires(
		waf.IPTypeAll,
		firewallconfigs.FirewallScopeServer,
		serverID,
		remoteAddr,
	)
	if !ok {
		t.Fatal("连接限制应写入 server scope 临时黑名单")
	}
	if expiresAt < before+edgecc.MaxConnectionsBlockSeconds-2 || expiresAt > time.Now().Unix()+edgecc.MaxConnectionsBlockSeconds+2 {
		t.Fatalf("第一次连接限制封禁应约为 %d 秒，expiresAt=%d before=%d", edgecc.MaxConnectionsBlockSeconds, expiresAt, before)
	}
	counter, ok := ccBlockedCounterMap.Get(remoteAddr)
	if !ok || counter.count != 1 {
		t.Fatalf("第一次连接限制封禁倍率应为 1，实际 %#v ok=%v", counter, ok)
	}
}

func TestHTTPRequestDoCCChecksMinQPSBeforeMaxConnections(t *testing.T) {
	const (
		serverID   int64 = 9_120_002
		clusterID  int64 = 7_202
		remoteAddr       = "198.51.100.122"
	)

	oldMap := ccBlockedCounterMap
	ccBlockedCounterMap = newCCBlockedCounterMap()
	defer func() { ccBlockedCounterMap = oldMap }()
	waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)
	defer waf.SharedIPBlackList.RemoveIP(remoteAddr, serverID, false)

	request, recorder := newCCMaxConnectionsTestRequest(t, serverID, clusterID, remoteAddr, 1, 1)
	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(qpsKey)

	conn := addCCMapTestConn(t, remoteAddr, 22101)
	for i := 1; i < 60; i++ {
		if request.doCC() {
			t.Fatalf("第 %d 次请求尚未达到一分钟平均 1 QPS，不应执行连接限制", i)
		}
	}
	if conn.closed {
		t.Fatal("MinQPS 门槛前不应关闭连接")
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != 59 {
		t.Fatalf("MinQPS 门槛前 QPS 计数应为 59，实际 %d", value)
	}

	if !request.doCC() {
		t.Fatal("第 60 次请求达到 MinQPS 后，已达到连接上限应触发 403")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("MinQPS 门槛后连接限制应返回 403，实际 %d", recorder.Code)
	}
	if !conn.closed {
		t.Fatal("达到 MinQPS 和连接上限后应关闭该 IP 连接")
	}
}

func newCCMaxConnectionsTestRequest(
	t *testing.T,
	serverID int64,
	clusterID int64,
	remoteAddr string,
	maxConnections int,
	minQPS int,
) (*HTTPRequest, *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest("GET", "https://example.com/cc-connections", nil)
	req.RemoteAddr = remoteAddr + ":12345"
	recorder := httptest.NewRecorder()
	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		Thresholds:           nil,
		MinQPSPerIP:          minQPS,
	}
	policy := nodeconfigs.NewHTTPCCPolicy()
	policy.MaxConnectionsPerIP = maxConnections
	policy.Firewall.Scope = firewallconfigs.FirewallScopeServer
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

func addCCMapTestConn(t *testing.T, remoteAddr string, port int) *ccMapTestConn {
	t.Helper()
	conn := &ccMapTestConn{
		remoteAddr: &net.TCPAddr{IP: net.ParseIP(remoteAddr), Port: port},
		localAddr:  &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 443},
	}
	conns.SharedMap.Add(conn)
	t.Cleanup(func() {
		conns.SharedMap.Remove(conn)
	})
	return conn
}

type ccMapTestConn struct {
	remoteAddr net.Addr
	localAddr  net.Addr
	closed     bool
}

func (c *ccMapTestConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *ccMapTestConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *ccMapTestConn) Close() error                       { c.closed = true; return nil }
func (c *ccMapTestConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *ccMapTestConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *ccMapTestConn) SetDeadline(_ time.Time) error      { return nil }
func (c *ccMapTestConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *ccMapTestConn) SetWriteDeadline(_ time.Time) error { return nil }
