package cc

import "github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"

// RedirectsCheckingEnabled 判断集群 CC 策略是否显式配置了完整的连续重定向检测参数。
//
// 这里不使用 DefaultHTTPCCPolicyRedirectsChecking* 常量做隐式回退，因为这些数值
// 虽然已经从 1.3.9 兼容模型中恢复，但当前审计记录仍要求在最终验收前继续核对。
// 只有策略实际下发完整参数时，节点运行时才应认为该检测已启用。
func RedirectsCheckingEnabled(policy *nodeconfigs.HTTPCCPolicy) bool {
	if policy == nil || !policy.IsOn {
		return false
	}
	config := policy.RedirectsChecking
	return config.ValidatePath != "" &&
		config.DurationSeconds > 0 &&
		config.MaxRedirects > 0 &&
		config.BlockSeconds > 0
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
