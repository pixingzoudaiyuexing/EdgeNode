package cc

import "github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"

const (
	// DefaultMaxConnectionsPerIP 是可信 1.3.9 Plus doCC() 在策略未配置正数时使用的默认值。
	DefaultMaxConnectionsPerIP = 30

	// MaxConnectionsBlockSeconds 是原版“单 IP 连接数达到上限”分支的基础封禁时长。
	// 可信原版静态反汇编中该分支把 increaseCCCounter() 返回的倍率乘以 0x708，
	// 即 1800 秒，然后再写入临时黑名单。
	MaxConnectionsBlockSeconds = 1800
)

// ResolveMaxConnectionsPerIP 返回 CC 实际使用的单 IP 最大连接数。
// 原版在 policy.MaxConnectionsPerIP > 0 时使用策略值，否则回退到 30。
func ResolveMaxConnectionsPerIP(policy *nodeconfigs.HTTPCCPolicy) int {
	if policy != nil && policy.IsOn && policy.MaxConnectionsPerIP > 0 {
		return policy.MaxConnectionsPerIP
	}
	return DefaultMaxConnectionsPerIP
}

// MaxConnectionsReached 判断当前连接数是否已经达到原版 CC 上限。
// 可信 1.3.9 Plus doCC() 比较关系是 count >= limit，而不是 count > limit。
func MaxConnectionsReached(count int, limit int) bool {
	if limit <= 0 {
		return false
	}
	return count >= limit
}
