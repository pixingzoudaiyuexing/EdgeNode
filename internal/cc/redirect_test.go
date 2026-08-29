package cc

import (
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
)

func TestRedirectsCheckingEnabledRequiresExplicitCompleteConfig(t *testing.T) {
	policy := nodeconfigs.NewHTTPCCPolicy()
	if RedirectsCheckingEnabled(policy) {
		t.Fatal("仅有构造默认值时不应隐式启用连续重定向检测")
	}

	policy.RedirectsChecking.ValidatePath = "/GE/CC/VALIDATOR"
	policy.RedirectsChecking.DurationSeconds = 120
	policy.RedirectsChecking.MaxRedirects = 30
	policy.RedirectsChecking.BlockSeconds = 3600
	if !RedirectsCheckingEnabled(policy) {
		t.Fatal("显式完整配置后应启用连续重定向检测")
	}

	policy.IsOn = false
	if RedirectsCheckingEnabled(policy) {
		t.Fatal("关闭集群 CC 策略后不应启用连续重定向检测")
	}
}

func TestRedirectsCheckingEnabledRejectsPartialConfig(t *testing.T) {
	policy := &nodeconfigs.HTTPCCPolicy{IsOn: true}
	policy.RedirectsChecking.ValidatePath = "/GE/CC/VALIDATOR"
	policy.RedirectsChecking.DurationSeconds = 120
	policy.RedirectsChecking.MaxRedirects = 30
	if RedirectsCheckingEnabled(policy) {
		t.Fatal("缺少 BlockSeconds 的不完整配置不应启用")
	}
}

func TestIsRedirectValidatorPathUsesExplicitPolicyPath(t *testing.T) {
	policy := &nodeconfigs.HTTPCCPolicy{IsOn: true}
	policy.RedirectsChecking.ValidatePath = "/custom/cc/validator"

	if !IsRedirectValidatorPath(policy, "/custom/cc/validator") {
		t.Fatal("应命中策略显式下发的 validator path")
	}
	if IsRedirectValidatorPath(policy, "/GE/CC/VALIDATOR") {
		t.Fatal("不应偷偷使用待核对的默认 validator path")
	}
	if IsRedirectValidatorPath(nil, "/custom/cc/validator") {
		t.Fatal("nil policy 不应命中")
	}
}
