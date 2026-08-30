//go:build plus

package nodes

import (
	"net/http"
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
)

// TestHTTPRequestDoCCLegacyCompatibilityFieldsDoNotAlterRuntime 固化 1.3.9 Plus 的兼容字段行为。
//
// 可信 1.3.9 Plus edge-node 静态审计确认：Level、IgnoreCommonAgents、Action 虽然存在于
// HTTPCCConfig 的运行时类型元数据中，但 doCC()、HTTPCCConfig.Init() 和 MatchURL() 都不会
// 将它们作为请求级 CC 开关读取。它们必须继续保留用于 JSON / 历史配置兼容，但不能在
// 自维护节点中擅自补成新的跳过、放行或阻断语义。
func TestHTTPRequestDoCCLegacyCompatibilityFieldsDoNotAlterRuntime(t *testing.T) {
	const (
		serverID   int64 = 9_140_001
		clusterID  int64 = 7_401
		remoteAddr       = "198.51.100.240"
		period            = 31
	)

	request, recorder := newCCThresholdBlockTestRequest(
		t,
		serverID,
		clusterID,
		remoteAddr,
		&serverconfigs.HTTPCCThreshold{
			PeriodSeconds: period,
			MaxRequests:   1,
			BlockSeconds:  0,
		},
	)

	// 使用明显非空的历史兼容字段，并附带常见浏览器 UA；原版 Node 请求链仍应只按照
	// 已恢复的真实运行时字段执行。关闭 GET302 是为了让本测试只验证普通阈值路径。
	request.web.CC.Level = "legacy-level"
	request.web.CC.IgnoreCommonAgents = true
	request.web.CC.Action = "legacy-action"
	request.web.CC.EnableGET302 = false
	request.RawReq.Header.Set("User-Agent", "Mozilla/5.0 AppleWebKit/537.36 Chrome/120 Safari/537.36")

	resetCCThresholdTestCounters(t, serverID, remoteAddr, request.RawReq.URL.Path, period)

	if !request.doCC() {
		t.Fatal("兼容字段非空时，已确认的请求阈值仍应正常阻断")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("兼容字段不应改变普通阈值的 429 行为，实际 HTTP %d", recorder.Code)
	}
	if !request.isAttack {
		t.Fatal("兼容字段不应阻止请求阈值设置 isAttack")
	}
	if !containsCCRequestTag(request.tags, ccThresholdTag) {
		t.Fatalf("兼容字段不应阻止请求阈值追加 %q 标签，实际 %#v", ccThresholdTag, request.tags)
	}
}
