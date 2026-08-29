package cc

const (
	// EscalationResetSeconds 是 1.3.9 Plus increaseCCCounter() 使用的重置间隔。
	// 可信原版静态反汇编可确认：若同一客户端 IP 距离上一次 CC 触发超过 86400 秒，
	// 计数会先清零，再把本次触发记为第 1 次。
	EscalationResetSeconds int64 = 24 * 60 * 60

	// EscalationMaxMultiplier 是 1.3.9 Plus 对重复 CC 触发倍率设置的上限。
	EscalationMaxMultiplier uint32 = 32
)

// NextEscalationMultiplier 计算同一客户端 IP 下一次 CC 触发时的封禁倍率。
//
// 原版 increaseCCCounter(remoteAddr) 的静态行为已经确认：
//   - 首次触发返回 1；
//   - 24 小时内再次触发时每次 +1；
//   - 最大为 32，不再继续增长；
//   - 距离上一次触发超过 24 小时，则从 1 重新开始；
//   - 每次调用都会把“最后触发时间”更新为当前时间。
//
// 这里保持为纯函数，只恢复已经确认的计数语义；原版节点使用 FixedMap 保存 IP 状态，
// 其容器容量尚未完成静态核对，因此本阶段不在这里猜测全局缓存容量。
func NextEscalationMultiplier(previous uint32, lastTriggeredAt int64, now int64) uint32 {
	if lastTriggeredAt < now-EscalationResetSeconds {
		previous = 0
	}

	if previous >= EscalationMaxMultiplier {
		return EscalationMaxMultiplier
	}
	return previous + 1
}
