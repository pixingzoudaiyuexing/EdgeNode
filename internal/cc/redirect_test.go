package cc

import (
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
)

func TestResolveRedirectLimitsUsesOriginalFallbacks(t *testing.T) {
	limits := ResolveRedirectLimits(nil)
	if limits.DurationSeconds != 120 || limits.MaxRedirects != 30 || limits.BlockSeconds != 3600 {
		t.Fatalf("nil policy 回退错误: %+v", limits)
	}

	policy := &nodeconfigs.HTTPCCPolicy{IsOn: true}
	policy.RedirectsChecking.DurationSeconds = 45
	policy.RedirectsChecking.MaxRedirects = 0
	policy.RedirectsChecking.BlockSeconds = 900
	limits = ResolveRedirectLimits(policy)
	if limits.DurationSeconds != 45 {
		t.Fatalf("显式 DurationSeconds 未生效: %+v", limits)
	}
	if limits.MaxRedirects != 30 {
		t.Fatalf("MaxRedirects 应独立回退到 30: %+v", limits)
	}
	if limits.BlockSeconds != 900 {
		t.Fatalf("显式 BlockSeconds 未生效: %+v", limits)
	}
}

func TestResolveRedirectLimitsDisabledPolicyKeepsHelperDefaults(t *testing.T) {
	policy := &nodeconfigs.HTTPCCPolicy{IsOn: false}
	policy.RedirectsChecking.DurationSeconds = 1
	policy.RedirectsChecking.MaxRedirects = 2
	policy.RedirectsChecking.BlockSeconds = 3

	limits := ResolveRedirectLimits(policy)
	if limits.DurationSeconds != 120 || limits.MaxRedirects != 30 || limits.BlockSeconds != 3600 {
		t.Fatalf("原版 helper 在关闭策略时应使用内部默认值: %+v", limits)
	}
}

func TestRedirectLimitReachedAtEquality(t *testing.T) {
	if RedirectLimitReached(29, 30) {
		t.Fatal("29/30 不应触发")
	}
	if !RedirectLimitReached(30, 30) {
		t.Fatal("30/30 应在等于阈值时立即触发")
	}
	if !RedirectLimitReached(31, 30) {
		t.Fatal("超过阈值应触发")
	}
	if RedirectLimitReached(1, 0) {
		t.Fatal("无效阈值不应触发")
	}
}

func TestRedirectsCheckingEnabledRequiresExplicitPath(t *testing.T) {
	policy := nodeconfigs.NewHTTPCCPolicy()
	if RedirectsCheckingEnabled(policy) {
		t.Fatal("尚未确认默认 validator path 接线前，不应无路径启用")
	}

	policy.RedirectsChecking.ValidatePath = "/GE/CC/VALIDATOR"
	if !RedirectsCheckingEnabled(policy) {
		t.Fatal("策略开启且显式提供 validator path 后应允许进入后续接线")
	}

	policy.IsOn = false
	if RedirectsCheckingEnabled(policy) {
		t.Fatal("关闭集群 CC 策略后不应启用")
	}
}

func TestIsRedirectValidatorPathUsesExplicitPolicyPath(t *testing.T) {
	policy := &nodeconfigs.HTTPCCPolicy{IsOn: true}
	policy.RedirectsChecking.ValidatePath = "/custom/cc/validator"

	if !IsRedirectValidatorPath(policy, "/custom/cc/validator") {
		t.Fatal("应命中策略显式下发的 validator path")
	}
	if IsRedirectValidatorPath(policy, "/GE/CC/VALIDATOR") {
		t.Fatal("默认 validator path 尚未接线前不应偷偷替换显式路径")
	}
	if IsRedirectValidatorPath(nil, "/custom/cc/validator") {
		t.Fatal("nil policy 不应命中")
	}
}
