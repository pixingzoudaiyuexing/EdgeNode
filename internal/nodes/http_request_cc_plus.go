//go:build plus

package nodes

import (
	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/iplibrary"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
	"github.com/iwind/TeaGo/Tea"
)

// doCC 执行 Plus 高级 CC 请求链中已经有 1.3.9 证据支持的筛选与统计部分。
//
// 当前阶段仍有意不执行“达到请求阈值后的 429 + 黑名单封禁”：
//   - MaxRequests 的等值触发、阈值顺序和重复触发倍率已经由可信 1.3.9 Plus 静态分析确认；
//   - 但原版 ccBlockedCounter 的 FixedMap 容量、GET302 validator 协议以及
//     MaxConnectionsPerIP 的完整接线仍在继续还原；
//   - 在这些运行时状态与旁路语义完全固定前，先保持请求阈值只观察/计数，避免半套封禁逻辑进入生产流量。
//
// 已确认并恢复的统计前顺序为：节点自动白名单 -> 系统 IP 名单 -> 当前 CC scope 的
// 临时黑名单 -> URL/QPS/阈值统计。名单拒绝和已在临时黑名单中的请求会立即关闭。
func (this *HTTPRequest) doCC() (block bool) {
	if this == nil || this.RawReq == nil || this.web == nil || this.web.CC == nil || !this.web.CC.IsOn {
		return false
	}
	if this.ReqServer == nil || this.ReqServer.Id <= 0 {
		return false
	}

	policy := this.findHTTPCCPolicy()
	// 可信 1.3.9 Plus doCC() 静态控制流确认：集群策略缺失时仍继续使用网站
	// 自定义 CC 配置；但策略对象明确存在且 IsOn=false 时会立即退出整个 CC 请求链。
	if policy != nil && !policy.IsOn {
		return false
	}

	config := edgecc.ResolveConfig(this.web.CC, policy)
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

	// 可信原版 doCC() 静态控制流确认：测试模式外先检查节点自动白名单。
	// 原版直接读取 NodeConfig 的 allowedIPMap；这里复用公开的等价方法，避免复制内部 map 细节。
	if !Tea.IsTesting() && this.nodeConfig != nil && this.nodeConfig.IPIsAutoAllowed(remoteAddr) {
		return false
	}

	// 系统 IP 名单与 WAF 使用同一 AllowIP() 语义：
	// canGoNext=false 表示名单动作已经要求终止；isAllowed=true 表示允许直通 CC。
	canGoNext, isAllowed, _ := iplibrary.AllowIP(remoteAddr, this.ReqServer.Id)
	if !canGoNext {
		this.isDone = true
		this.Close()
		return true
	}
	if isAllowed {
		return false
	}

	// 原版随后检查 SharedIPBlackList，而不是 WhiteList。Contains() 的参数由可信
	// doCC 汇编和当前 IPList ABI 交叉确认：IPTypeAll + CC FirewallScope + serverId + IP。
	// 集群策略没有给 scope 时原版回退为 global。
	firewallScope := firewallconfigs.FirewallScopeGlobal
	if policy != nil {
		firewallScope = policy.FirewallScope()
	}
	if waf.SharedIPBlackList.Contains(waf.IPTypeAll, firewallScope, this.ReqServer.Id, remoteAddr) {
		this.isDone = true
		this.Close()
		return true
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
		// 当前仍只记录；ThresholdReached() 已独立恢复并测试，待封禁状态容器接好后
		// 再把第一条命中的阈值接入 429/RecordIP。
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
