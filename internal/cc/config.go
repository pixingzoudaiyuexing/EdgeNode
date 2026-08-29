package cc

import (
	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
)

// ResolveConfig 根据网站 CC 配置和集群 CC 策略生成一次请求可安全使用的运行时配置副本。
//
// 1. 网站选择“使用默认阈值”时，优先使用当前集群策略阈值；策略缺失或阈值为空时，
//    回退到 1.3.9 配置模型中已经确认的默认阈值。
// 2. 网站选择“自定义阈值”时，严格保留网站阈值；空自定义配置也不擅自替换为默认值。
// 3. 不修改 site / policy 中任何共享对象，避免并发请求之间相互污染。
// 4. Level、Fingerprint、GET302 等高级字段目前只原样保留，等各自运行语义有证据后再处理。
func ResolveConfig(site *serverconfigs.HTTPCCConfig, policy *nodeconfigs.HTTPCCPolicy) *serverconfigs.HTTPCCConfig {
	if site == nil {
		return nil
	}

	resolved := *site

	if site.UseDefaultThresholds {
		if policy != nil && len(policy.Thresholds) > 0 {
			resolved.Thresholds = serverconfigs.CloneHTTPCCThresholds(policy.Thresholds)
		} else {
			resolved.Thresholds = serverconfigs.CloneHTTPCCThresholds(serverconfigs.DefaultHTTPCCThresholds)
		}
	} else {
		resolved.Thresholds = serverconfigs.CloneHTTPCCThresholds(site.Thresholds)
	}

	// 历史默认配置明确以 block 作为空 Action 的兼容回退值。
	if resolved.Action == "" && serverconfigs.DefaultHTTPCCConfig != nil {
		resolved.Action = serverconfigs.DefaultHTTPCCConfig.Action
	}

	return &resolved
}
