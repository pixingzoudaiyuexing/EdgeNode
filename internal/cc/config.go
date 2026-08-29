package cc

import (
	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
)

// ResolveConfig 根据网站 CC 配置和集群 CC 策略生成一次请求可安全使用的运行时配置副本。
//
// 1. 网站选择“使用默认阈值”时，只使用当前集群策略明确下发的阈值；策略缺失或阈值为空时保持为空。
//    1.3.9 Plus 的默认阈值具体数值仍待最终静态/行为证据确认，节点运行时不能自行启用候选默认值。
// 2. 网站选择“自定义阈值”时，严格保留网站阈值；空自定义配置也不擅自替换为默认值。
// 3. 不修改 site / policy 中任何共享对象，避免并发请求之间相互污染。
// 4. Level、Action、EnableFingerprint、EnableGET302 等高级字段目前只原样保留，
//    等各自运行语义有证据后再处理。
func ResolveConfig(site *serverconfigs.HTTPCCConfig, policy *nodeconfigs.HTTPCCPolicy) *serverconfigs.HTTPCCConfig {
	if site == nil {
		return nil
	}

	resolved := *site

	if site.UseDefaultThresholds {
		if policy != nil && len(policy.Thresholds) > 0 {
			resolved.Thresholds = cloneThresholds(policy.Thresholds)
		} else {
			resolved.Thresholds = nil
		}
	} else {
		resolved.Thresholds = cloneThresholds(site.Thresholds)
	}

	return &resolved
}

func cloneThresholds(thresholds []*serverconfigs.HTTPCCThreshold) []*serverconfigs.HTTPCCThreshold {
	if len(thresholds) == 0 {
		return nil
	}

	cloned := make([]*serverconfigs.HTTPCCThreshold, 0, len(thresholds))
	for _, threshold := range thresholds {
		cloned = append(cloned, threshold.Clone())
	}
	return cloned
}
