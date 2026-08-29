//go:build plus

package nodes

import (
	"net/http/httptest"
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
)

func TestHTTPRequestDoCCSkipsWhenClusterPolicyExplicitlyDisabled(t *testing.T) {
	const (
		serverID   int64 = 9_100_101
		clusterID  int64 = 7_101
		remoteAddr       = "198.51.100.101"
	)

	req := httptest.NewRequest("GET", "https://example.com/disabled-policy", nil)
	req.RemoteAddr = remoteAddr + ":12345"

	config := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: false,
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: 60,
			MaxRequests:   1,
			BlockSeconds:  600,
		}},
	}
	policy := &nodeconfigs.HTTPCCPolicy{IsOn: false}
	nodeConfig := &nodeconfigs.NodeConfig{
		HTTPCCPolicies: map[int64]*nodeconfigs.HTTPCCPolicy{clusterID: policy},
	}

	qpsKey := edgecc.QPSCounterKey(serverID, remoteAddr)
	counters.SharedCounter.ResetKey(qpsKey)
	defer counters.SharedCounter.ResetKey(qpsKey)

	request := &HTTPRequest{
		RawReq: req,
		ReqServer: &serverconfigs.ServerConfig{
			Id:        serverID,
			ClusterId: clusterID,
		},
		nodeConfig: nodeConfig,
		web:        &serverconfigs.HTTPWebConfig{CC: config},
	}

	if request.doCC() {
		t.Fatal("集群 CC 策略明确关闭时不应阻断请求")
	}
	if value := counters.SharedCounter.GetKey(qpsKey); value != 0 {
		t.Fatalf("集群 CC 策略明确关闭时不应进入 QPS 统计，实际 %d", value)
	}
}
