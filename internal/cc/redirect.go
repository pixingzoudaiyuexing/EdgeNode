package cc

import "github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"

// RedirectLimits 是连续重定向检测实际使用的三个数值参数。
type RedirectLimits struct {
	DurationSeconds int
	MaxRedirects    int
	BlockSeconds    int
}

// ResolveRedirectLimits 按 1.3.9 Plus checkCCRedirects() 的原版行为解析参数。
//
// 可信原版静态反汇编已经确认：
//   - DurationSeconds <= 0 时回退到 120；
//   - MaxRedirects <= 0 时回退到 30；
//   - BlockSeconds <= 0 时回退到 3600；
//   - 三个字段是分别回退，并不要求必须同时显式配置。
//
// 原版 helper 在 policy 为 nil 或 IsOn=false 时同样使用这组三个默认值；
// “是否应该进入连续重定向检测”由 doCC 上层请求流程决定，本函数只恢复 helper
// 内部已经确认的参数解析语义，不把启用条件混在这里。
func ResolveRedirectLimits(policy *nodeconfigs.HTTPCCPolicy) RedirectLimits {
	limits := RedirectLimits{
		DurationSeconds: nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingDurationSeconds,
		MaxRedirects:    nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingMaxRedirects,
		BlockSeconds:    nodeconfigs.DefaultHTTPCCPolicyRedirectsCheckingBlockSeconds,
	}
	if policy == nil || !policy.IsOn {
		return limits
	}

	config := policy.RedirectsChecking
	if config.DurationSeconds > 0 {
		limits.DurationSeconds = config.DurationSeconds
	}
	if config.MaxRedirects > 0 {
		limits.MaxRedirects = config.MaxRedirects
	}
	if config.BlockSeconds > 0 {
		limits.BlockSeconds = config.BlockSeconds
	}
	return limits
}

// RedirectLimitReached 判断连续重定向次数是否命中封禁阈值。
// 原版 checkCCRedirects() 在 Counter.IncreaseKey() 返回值 >= MaxRedirects 时封禁，
// 因此第 N 次请求在计数恰好等于 N 时就会触发，而不是等待第 N+1 次。
func RedirectLimitReached(count uint32, maxRedirects int) bool {
	if maxRedirects <= 0 {
		return false
	}
	return uint64(count) >= uint64(maxRedirects)
}

// RedirectsCheckingEnabled 判断集群 CC 策略是否显式配置了 validator 路径。
//
// 数值参数的原版回退语义已经由 ResolveRedirectLimits() 恢复；但 validator path
// 在 doCC 中的默认路径和 key 校验流程仍在继续做静态还原，因此当前这里只把
// “策略已开启且明确提供 validatePath”作为运行时接线的保守启用条件。
func RedirectsCheckingEnabled(policy *nodeconfigs.HTTPCCPolicy) bool {
	return policy != nil && policy.IsOn && policy.RedirectsChecking.ValidatePath != ""
}

// IsRedirectValidatorPath 判断当前请求 Path 是否命中策略显式下发的 CC validator 路径。
// 本函数只负责路径识别，不校验 key 参数，也不记录重定向次数。
func IsRedirectValidatorPath(policy *nodeconfigs.HTTPCCPolicy, requestPath string) bool {
	if policy == nil || requestPath == "" {
		return false
	}
	validatePath := policy.RedirectsChecking.ValidatePath
	return validatePath != "" && requestPath == validatePath
}
