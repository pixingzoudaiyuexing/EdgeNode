//go:build plus

package nodes

import (
	"net/http/httptest"
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/shared"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
)

func TestHTTPRequestDoCCObservesCustomThreshold(t *testing.T) {
	const (
		serverID   int64 = 9_100_001
		remoteAddr       = "198.51.100.60"
		period            = 60
	)

	req := httptest.NewRequest("GET", "https://example.com/protected", nil)
	req.RemoteAddr = remoteAddr + ":12345"

	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: period,
			MaxRequests:   10,
			BlockSeconds:  600,
		}},
	}

	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	thresholdKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, req.URL.Path, false, period)
	counters.SharedCounter.ResetKey(qpsKey)
	counters.SharedCounter.ResetKey(thresholdKey)
	defer counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(thresholdKey)

	request := &HTTPRequest{
		RawReq: req,
		ReqServer: &serverconfigs.ServerConfig{
			Id: serverID,
		},
		web: &serverconfigs.HTTPWebConfig{CC: config},
	}

	if request.doCC() {
		t.Fatal("当前观察阶段不应阻断请求")
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != 1 {
		t.Fatalf("QPS 计数应为 1，实际 %d", value)
	}
	if value := counters.SharedCounter.GetKey(thresholdKey); value != 1 {
		t.Fatalf("阈值计数应为 1，实际 %d", value)
	}
}

func TestHTTPRequestDoCCSkipsCommonFileBeforeCounting(t *testing.T) {
	const (
		serverID   int64 = 9_100_002
		remoteAddr       = "198.51.100.61"
		period            = 60
	)

	req := httptest.NewRequest("GET", "https://example.com/app.js", nil)
	req.RemoteAddr = remoteAddr + ":23456"
	req.Header.Set("Referer", "https://example.com/")

	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		IgnoreCommonFiles:    true,
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: period,
			MaxRequests:   10,
			BlockSeconds:  600,
		}},
	}

	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	thresholdKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, req.URL.Path, false, period)
	counters.SharedCounter.ResetKey(qpsKey)
	counters.SharedCounter.ResetKey(thresholdKey)
	defer counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(thresholdKey)

	request := &HTTPRequest{
		RawReq: req,
		ReqServer: &serverconfigs.ServerConfig{
			Id: serverID,
		},
		web: &serverconfigs.HTTPWebConfig{CC: config},
	}

	if request.doCC() {
		t.Fatal("常见文件跳过时不应阻断请求")
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != 0 {
		t.Fatalf("常见文件应在 QPS 计数前跳过，实际 QPS 计数 %d", value)
	}
	if value := counters.SharedCounter.GetKey(thresholdKey); value != 0 {
		t.Fatalf("常见文件不应进入阈值统计，实际 %d", value)
	}
}

func TestHTTPRequestDoCCWaitsForMinQPS(t *testing.T) {
	const (
		serverID   int64 = 9_100_003
		remoteAddr       = "198.51.100.62"
		period            = 60
	)

	req := httptest.NewRequest("GET", "https://example.com/api", nil)
	req.RemoteAddr = remoteAddr + ":34567"
	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		MinQPSPerIP:          1,
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: period,
			MaxRequests:   10,
			BlockSeconds:  600,
		}},
	}

	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	thresholdKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, req.URL.Path, false, period)
	counters.SharedCounter.ResetKey(qpsKey)
	counters.SharedCounter.ResetKey(thresholdKey)
	defer counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(thresholdKey)

	request := &HTTPRequest{
		RawReq:     req,
		ReqServer: &serverconfigs.ServerConfig{Id: serverID},
		web:       &serverconfigs.HTTPWebConfig{CC: config},
	}

	for i := 0; i < 59; i++ {
		if request.doCC() {
			t.Fatal("MinQPS 门槛前不应阻断请求")
		}
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != 59 {
		t.Fatalf("QPS 计数应为 59，实际 %d", value)
	}
	if value := counters.SharedCounter.GetKey(thresholdKey); value != 0 {
		t.Fatalf("未达到一分钟平均 1 QPS 前不应进入阈值统计，实际 %d", value)
	}

	if request.doCC() {
		t.Fatal("当前观察阶段不应阻断请求")
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != 60 {
		t.Fatalf("第 60 次请求后 QPS 计数应为 60，实际 %d", value)
	}
	if value := counters.SharedCounter.GetKey(thresholdKey); value != 1 {
		t.Fatalf("达到一分钟平均 1 QPS 后应开始阈值统计，实际 %d", value)
	}
}

func TestHTTPRequestDoCCAppliesOnlyAndExceptURLBeforeCounting(t *testing.T) {
	const (
		serverID   int64 = 9_100_004
		remoteAddr       = "198.51.100.63"
		period            = 60
	)

	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		OnlyURLPatterns: []*shared.URLPattern{{
			Type: shared.URLPatternTypeWildcard, Pattern: "/search*",
		}},
		ExceptURLPatterns: []*shared.URLPattern{{
			Type: shared.URLPatternTypeWildcard, Pattern: "/search/api*",
		}},
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: period,
			MaxRequests:   10,
			BlockSeconds:  600,
		}},
	}
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	thresholdKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, "/search?q=ok", false, period)
	counters.SharedCounter.ResetKey(qpsKey)
	counters.SharedCounter.ResetKey(thresholdKey)
	defer counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(thresholdKey)

	makeRequest := func(rawURL string) *HTTPRequest {
		req := httptest.NewRequest("GET", rawURL, nil)
		req.RemoteAddr = remoteAddr + ":45678"
		return &HTTPRequest{
			RawReq:     req,
			ReqHost:    req.Host,
			ReqServer: &serverconfigs.ServerConfig{Id: serverID},
			web:       &serverconfigs.HTTPWebConfig{CC: config},
		}
	}

	makeRequest("https://example.com/index").doCC()
	makeRequest("https://example.com/search/api/list").doCC()
	if value := counters.SharedCounter.GetKey(qpsKey); value != 0 {
		t.Fatalf("Only/Except 跳过的请求不应进入 QPS 统计，实际 %d", value)
	}

	makeRequest("https://example.com/search?q=ok").doCC()
	if value := counters.SharedCounter.GetKey(qpsKey); value != 1 {
		t.Fatalf("Only URL 命中的请求应进入 QPS 统计，实际 %d", value)
	}
}

func TestHTTPRequestDoCCUsesClusterThresholdsWhenExplicitlyConfigured(t *testing.T) {
	const (
		serverID  int64 = 9_100_005
		clusterID int64 = 7_001
		period          = 17
	)
	remoteAddr := "198.51.100.64"
	req := httptest.NewRequest("GET", "https://example.com/cluster", nil)
	req.RemoteAddr = remoteAddr + ":56789"

	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: true,
	}
	policy := &nodeconfigs.HTTPCCPolicy{
		IsOn: true,
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: period,
			MaxRequests:   4,
			BlockSeconds:  300,
		}},
	}
	nodeConfig := &nodeconfigs.NodeConfig{
		HTTPCCPolicies: map[int64]*nodeconfigs.HTTPCCPolicy{clusterID: policy},
	}

	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	thresholdKey := edgecc.ThresholdCounterKey(serverID, remoteAddr, req.URL.Path, false, period)
	counters.SharedCounter.ResetKey(qpsKey)
	counters.SharedCounter.ResetKey(thresholdKey)
	defer counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(thresholdKey)

	request := &HTTPRequest{
		RawReq:     req,
		ReqServer: &serverconfigs.ServerConfig{Id: serverID, ClusterId: clusterID},
		nodeConfig: nodeConfig,
		web:       &serverconfigs.HTTPWebConfig{CC: config},
	}
	request.doCC()

	if value := counters.SharedCounter.GetKey(thresholdKey); value != 1 {
		t.Fatalf("明确下发的集群阈值应进入统计，实际 %d", value)
	}
}
