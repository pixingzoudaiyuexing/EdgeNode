//go:build plus

package nodes

import (
	"net/http/httptest"
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
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
