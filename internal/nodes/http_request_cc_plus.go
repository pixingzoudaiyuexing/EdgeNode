//go:build plus

package nodes

import (
	"fmt"
	"net/http"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/conns"
	"github.com/TeaOSLab/EdgeNode/internal/iplibrary"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
	"github.com/iwind/TeaGo/Tea"
)

const (
	ccThresholdTag        = "CCProtection"
	ccTooManyRequestsEN   = "Too many requests, please wait for a few minutes."
	ccTooManyRequestsZHCN = "访问过于频繁，请稍等片刻后再访问。"
	ccConnections403EN    = "The request has been blocked by cc policy."
	ccConnections403ZHCN  = "当前请求已被CC策略拦截。"
)

// doCC 执行 Plus 高级 CC 请求链中已经由 1.3.9 证据确认的筛选、统计和封禁逻辑。
//
// 当前已经恢复：
//   - 节点自动白名单、系统 IP 名单和 CC 临时黑名单的原版检查顺序；
//   - URL/常见静态文件/MinQPSPerIP 过滤；
//   - 单 IP 最大连接数检查，默认 30，count >= limit 时触发 403；
//   - 来源 IP + 可选 HTTPS 指纹的多周期阈值统计；
//   - count >= MaxRequests 时第 N 次请求立即触发 429；
//   - 两类封禁共用按客户端 IP 计算的 24 小时、最高 32 倍重复封禁递增；
//   - FirewallScope、临时黑名单、本机防火墙范围，以及请求阈值的 CCProtection 标签/攻击标记。
//
// GET302 validator 仍由后续独立阶段接入，避免把尚未完整验证的浏览器重定向协议
// 混进已经可以逐项验收的连接限制和普通请求阈值路径。
func (this *HTTPRequest) doCC() (block bool) {
	if this == nil || this.RawReq == nil || this.web == nil || this.web.CC == nil || !this.web.CC.IsOn {
		return false
	}
	if this.ReqServer == nil || this.ReqServer.Id <= 0 {
		return false
	}

	policy := this.findHTTPCCPolicy()
	// 可信 1.3.9 Plus 控制流确认：集群没有下发 policy 时网站自定义 CC 仍可继续；
	// 但 policy 明确存在且 IsOn=false 时整个 CC 请求链立即退出。
	if policy != nil && !policy.IsOn {
		return false
	}

	config := edgecc.ResolveConfig(this.web.CC, policy)
	if config == nil {
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
		this.Close()
		return true
	}

	// MinQPSPerIP 的官方语义明确为“一分钟平均 QPS 达到设定值”。
	// 原版先通过这个门槛，再检查 MaxConnectionsPerIP 和后续 GET302/请求阈值。
	requestsLastMinute := edgecc.IncreaseQPS(this.ReqServer.Id, remoteAddr)
	if !edgecc.ReachedMinQPS(config.MinQPSPerIP, int64(requestsLastMinute)) {
		return false
	}

	// 单 IP 最大连接数直接使用连接接入时维护的 conns.SharedMap，不调用 ClientConn.Bind()。
	// 可信 1.3.9 Plus 行为：policy 缺失或配置 <=0 时上限为 30；count >= limit 的
	// 第 N 条连接即触发 403，并固定按 1800 秒基础时长乘同一个重复封禁倍率。
	maxConnectionsPerIP := edgecc.ResolveMaxConnectionsPerIP(policy)
	if edgecc.MaxConnectionsReached(conns.SharedMap.CountIPConns(remoteAddr), maxConnectionsPerIP) {
		this.writeCode(http.StatusForbidden, ccConnections403EN, ccConnections403ZHCN)

		multiplier := this.increaseCCCounter(remoteAddr)
		blockSeconds := int(int32(edgecc.MaxConnectionsBlockSeconds) * multiplier)
		reason := fmt.Sprintf("CC防护拦截：并发连接数超出%d个", maxConnectionsPerIP)
		edgecc.RecordBlockedIP(
			this.ReqServer.Id,
			remoteAddr,
			firewallScope,
			blockSeconds,
			firewallScope == firewallconfigs.FirewallScopeGlobal,
			reason,
		)
		// 原版此分支不追加 CCProtection/isAttack，也不额外调用 HTTPRequest.Close()；
		// RecordIP() 会关闭该 IP 当前所有已登记连接。
		return true
	}

	fingerprint := this.WAFFingerprint()
	for _, threshold := range config.Thresholds {
		if threshold == nil || threshold.PeriodSeconds <= 0 || threshold.MaxRequests <= 0 {
			continue
		}

		// 1.3.9 原生 Counter 会按统计周期维护时间分片；指纹开启时同时统计
		// 来源 IP 与 HTTPS 连接指纹，并由基础层返回两者中的较大值。
		count := edgecc.IncreaseThresholdWithFingerprint(
			this.ReqServer.Id,
			remoteAddr,
			requestPath,
			config.WithRequestPath,
			int(threshold.PeriodSeconds),
			config.EnableFingerprint,
			fingerprint,
		)
		if !edgecc.ThresholdReached(count, threshold.MaxRequests) {
			continue
		}

		// 可信 1.3.9 Plus 汇编确认：第一个命中的阈值立即返回 429，后续阈值不再继续检查。
		// 即使 BlockSeconds<=0，也仍然返回 429；只有真正配置了封禁时长才写入临时黑名单。
		this.writeCode(http.StatusTooManyRequests, ccTooManyRequestsEN, ccTooManyRequestsZHCN)

		if threshold.BlockSeconds > 0 {
			multiplier := this.increaseCCCounter(remoteAddr)
			// 原版在 amd64 上使用 32 位有符号乘法；两个源字段也都是 int32。
			// 保持相同的 int32 乘法边界，再转换成 RecordBlockedIP 接口使用的 int。
			blockSeconds := int(threshold.BlockSeconds * multiplier)
			reason := fmt.Sprintf(
				"CC防护拦截：在%d秒内请求超过%d次",
				threshold.PeriodSeconds,
				threshold.MaxRequests,
			)
			edgecc.RecordBlockedIP(
				this.ReqServer.Id,
				remoteAddr,
				firewallScope,
				blockSeconds,
				firewallScope == firewallconfigs.FirewallScopeGlobal,
				reason,
			)
		}

		// 原版在阈值命中后追加固定 12 字节标签并设置 isAttack=true。
		// GoReSym/ELF 字符串和当前 HTTPRequest 结构偏移已经交叉确认这两个字段。
		this.tags = append(this.tags, ccThresholdTag)
		this.isAttack = true

		this.Close()
		// RecordIP() 自身会关闭同 IP 连接，但原版 doCC() 仍再次显式调用一次；
		// 保留该调用以匹配 1.3.9 Plus 的请求终止行为。
		conns.SharedMap.CloseIPConns(remoteAddr)
		return true
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
