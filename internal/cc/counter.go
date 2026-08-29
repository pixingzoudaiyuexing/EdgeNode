package cc

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
)

const counterPrefix = "ADV-CC"

// ThresholdCounterKey 生成高级 CC 某个统计周期对应的计数 key。
//
// 1.3.9 原生 Counter 会在 key 第一次创建时固定 lifeSeconds，因此 5s/60s/300s
// 必须使用不同 key，不能只改 IncreaseKey() 的 period 参数复用同一个 key。
// serverId 用于隔离不同网站；withRequestPath 开启时再按请求 Path 隔离。
func ThresholdCounterKey(serverId int64, clientKey string, requestPath string, withRequestPath bool, periodSeconds int) string {
	var builder strings.Builder
	builder.WriteString(counterPrefix)
	builder.WriteString(":threshold:")
	builder.WriteString(strconv.FormatInt(serverId, 10))
	builder.WriteByte(':')
	builder.WriteString(strconv.Itoa(periodSeconds))
	builder.WriteByte(':')
	builder.WriteString(clientKey)
	if withRequestPath {
		builder.WriteByte(':')
		builder.WriteString(requestPath)
	}
	return builder.String()
}

// QPSCounterKey 生成单 IP 一分钟 QPS 门槛的计数 key。
// MinQPSPerIP 的官方语义是网站内单 IP 一分钟平均 QPS，因此不跟随
// WithRequestPath 拆分，避免把“单 IP QPS”误变成“单 IP 单路径 QPS”。
func QPSCounterKey(serverId int64, remoteAddr string) string {
	return counterPrefix + ":qps:" + strconv.FormatInt(serverId, 10) + ":" + remoteAddr
}

// RedirectCounterKey 生成 CC GET302 连续重定向检测的计数 key。
//
// HTTPCCPolicy.RedirectsChecking 的统计周期可以配置；与普通阈值计数相同，
// period 必须进入 key，避免同一 IP 在策略热更新后复用旧 Counter 生命周期。
func RedirectCounterKey(serverId int64, remoteAddr string, durationSeconds int) string {
	return counterPrefix + ":redirect:" + strconv.FormatInt(serverId, 10) + ":" + strconv.Itoa(durationSeconds) + ":" + remoteAddr
}

// ThresholdReached 判断一次已经完成 IncreaseKey() 的请求是否达到 CC 阈值。
//
// 可信 1.3.9 Plus doCC() 反汇编已经确认原版比较关系：
// 当当前计数 count >= MaxRequests 时立即触发。因此 MaxRequests=30 时，
// 第 30 次请求就进入 CC 动作，不需要等到第 31 次。
func ThresholdReached(count uint32, maxRequests int32) bool {
	if maxRequests <= 0 {
		return false
	}
	return uint64(count) >= uint64(maxRequests)
}

// IncreaseThreshold 使用 1.3.9 原生时间分片 Counter 记录一次阈值请求。
func IncreaseThreshold(serverId int64, clientKey string, requestPath string, withRequestPath bool, periodSeconds int) uint32 {
	if serverId <= 0 || clientKey == "" || periodSeconds <= 0 {
		return 0
	}
	key := ThresholdCounterKey(serverId, clientKey, requestPath, withRequestPath, periodSeconds)
	return counters.SharedCounter.IncreaseKey(key, periodSeconds)
}

// IncreaseThresholdWithFingerprint 同时按来源 IP 和 HTTPS 连接指纹统计，并返回较大值。
//
// 1.3.9 的 WAF cc2 已公开确认使用同样策略：先统计 remoteAddr，再将连接指纹
// 格式化为十六进制作为第二个 client key 独立计数，最终取两者最大值。这样同一代理
// 出口下不断变化 IP / 客户端来源时，指纹统计仍可提供额外识别能力。
func IncreaseThresholdWithFingerprint(serverId int64, remoteAddr string, requestPath string, withRequestPath bool, periodSeconds int, enableFingerprint bool, fingerprint []byte) uint32 {
	ipValue := IncreaseThreshold(serverId, remoteAddr, requestPath, withRequestPath, periodSeconds)
	if !enableFingerprint || len(fingerprint) == 0 {
		return ipValue
	}

	fingerprintValue := IncreaseThreshold(serverId, hex.EncodeToString(fingerprint), requestPath, withRequestPath, periodSeconds)
	if fingerprintValue > ipValue {
		return fingerprintValue
	}
	return ipValue
}

// IncreaseQPS 使用同一套 1.3.9 原生 Counter 记录单 IP 最近一分钟请求数。
func IncreaseQPS(serverId int64, remoteAddr string) uint32 {
	if serverId <= 0 || remoteAddr == "" {
		return 0
	}
	return counters.SharedCounter.IncreaseKey(QPSCounterKey(serverId, remoteAddr), 60)
}

// IncreaseRedirect 记录一次 CC GET302 重定向行为。
func IncreaseRedirect(serverId int64, remoteAddr string, durationSeconds int) uint32 {
	if serverId <= 0 || remoteAddr == "" || durationSeconds <= 0 {
		return 0
	}
	return counters.SharedCounter.IncreaseKey(RedirectCounterKey(serverId, remoteAddr, durationSeconds), durationSeconds)
}
