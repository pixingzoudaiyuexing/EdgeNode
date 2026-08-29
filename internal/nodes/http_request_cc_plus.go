//go:build plus

package nodes

import (
	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
)

// doCC 执行 Plus 高级 CC 请求链中已经有 1.3.9 证据支持的筛选与统计部分。
//
// 当前阶段有意只做“观察/计数”，不会因为统计值直接封禁请求：
//   - MaxRequests 到底在第 N 次还是第 N+1 次触发仍缺直接证据；
//   - 多个时间窗口同时命中时 BlockSeconds 的选择规则仍待确认；
//   - MaxConnectionsPerIP 与现有 RequestLimit 共用 ClientConn.Bind，不能抢先绑定；
//   - GET302 的 /GE/CC/VALIDATOR key 生成/校验协议尚未恢复。
//
// 在这些边界确认前，本函数始终返回 false，避免“为了补齐功能”改变生产流量行为。
func (this *HTTPRequest) doCC() (block bool) {
	if this == nil || this.RawReq == nil || this.web == nil || this.web.CC == nil || !this.web.CC.IsOn {
		return false
	}
	if this.ReqServer == nil || this.ReqServer.Id <= 0 {
		return false
	}

	config := edgecc.ResolveConfig(this.web.CC, this.findHTTPCCPolicy())
	if config == nil || len(config.Thresholds) == 0 {
		return false
	}

	// Only / Except URL 使用 EdgeCommon 已恢复的同代匹配逻辑。
	if !edgecc.MatchURL(config, this.URL()) {
		return false
	}

	requestPath := this.RawReq.URL.Path
	if edgecc.ShouldIgnoreCommonFile(config, requestPath, this.RawReq.Referer()) {
		return false
	}

	remoteAddr := this.requestRemoteAddr(true)
	if remoteAddr == "" {
		return false
	}

	// MinQPSPerIP 的官方语义明确为“一分钟平均 QPS 达到设定值”。
	// 因此这里可以安全确定 >= 边界；0 表示不设置最低门槛。
	requestsLastMinute := edgecc.IncreaseQPS(this.ReqServer.Id, remoteAddr)
	if !edgecc.ReachedMinQPS(config.MinQPSPerIP, int64(requestsLastMinute)) {
		return false
	}

	fingerprint := this.WAFFingerprint()
	for _, threshold := range config.Thresholds {
		if threshold == nil || threshold.PeriodSeconds <= 0 || threshold.MaxRequests <= 0 {
			continue
		}

		// 1.3.9 原生 Counter 会按统计周期维护时间分片；指纹开启时同时统计
		// 来源 IP 与 HTTPS 连接指纹，并由基础层返回两者中的较大值。
		// 当前只记录，不解释 MaxRequests 的最终触发边界。
		_ = edgecc.IncreaseThresholdWithFingerprint(
			this.ReqServer.Id,
			remoteAddr,
			requestPath,
			config.WithRequestPath,
			int(threshold.PeriodSeconds),
			config.EnableFingerprint,
			fingerprint,
		)
	}

	return false
}

// findHTTPCCPolicy 只读取当前集群实际下发的策略。
// 不在节点端构造候选默认策略，避免把尚未最终核实的默认数值变成运行时行为。
func (this *HTTPRequest) findHTTPCCPolicy() *nodeconfigs.HTTPCCPolicy {
	if this == nil || this.nodeConfig == nil || this.ReqServer == nil {
		return nil
	}
	return this.nodeConfig.FindHTTPCCPolicyWithClusterId(this.ReqServer.ClusterId)
}
