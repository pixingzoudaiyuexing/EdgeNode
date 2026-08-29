package cc

import (
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

// IncreaseThreshold 使用 1.3.9 原生时间分片 Counter 记录一次阈值请求。
// 本函数只返回当前统计值，不解释 MaxRequests 的触发边界。
func IncreaseThreshold(serverId int64, clientKey string, requestPath string, withRequestPath bool, periodSeconds int) uint32 {
	if serverId <= 0 || clientKey == "" || periodSeconds <= 0 {
		return 0
	}
	key := ThresholdCounterKey(serverId, clientKey, requestPath, withRequestPath, periodSeconds)
	return counters.SharedCounter.IncreaseKey(key, periodSeconds)
}

// IncreaseQPS 使用同一套 1.3.9 原生 Counter 记录单 IP 最近一分钟请求数。
func IncreaseQPS(serverId int64, remoteAddr string) uint32 {
	if serverId <= 0 || remoteAddr == "" {
		return 0
	}
	return counters.SharedCounter.IncreaseKey(QPSCounterKey(serverId, remoteAddr), 60)
}
