package cc

import (
	"reflect"
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
)

func TestResolveConfigWithPolicyThresholds(t *testing.T) {
	policyThresholds := []*serverconfigs.HTTPCCThreshold{{
		PeriodSeconds: 10,
		MaxRequests:   20,
		BlockSeconds:  30,
	}}
	site := &serverconfigs.HTTPCCConfig{
		IsOn:                 true,
		UseDefaultThresholds: true,
		Action:                "",
	}
	policy := &nodeconfigs.HTTPCCPolicy{Thresholds: policyThresholds}

	resolved := ResolveConfig(site, policy)
	if resolved == nil {
		t.Fatal("resolved should not be nil")
	}
	if !reflect.DeepEqual(resolved.Thresholds, policyThresholds) {
		t.Fatalf("unexpected thresholds: %#v", resolved.Thresholds)
	}
	if resolved.Action != serverconfigs.DefaultHTTPCCConfig.Action {
		t.Fatalf("unexpected action: %q", resolved.Action)
	}

	// 必须是独立副本，运行时修改不能污染策略对象。
	resolved.Thresholds[0].MaxRequests = 999
	if policyThresholds[0].MaxRequests == 999 {
		t.Fatal("resolved thresholds should not share threshold objects with policy")
	}
}

func TestResolveConfigWithCustomThresholds(t *testing.T) {
	customThresholds := []*serverconfigs.HTTPCCThreshold{{
		PeriodSeconds: 3,
		MaxRequests:   7,
		BlockSeconds:  60,
	}}
	site := &serverconfigs.HTTPCCConfig{
		UseDefaultThresholds: false,
		Thresholds:           customThresholds,
		Action:                "block",
	}
	policy := &nodeconfigs.HTTPCCPolicy{
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: 100,
			MaxRequests:   200,
			BlockSeconds:  300,
		}},
	}

	resolved := ResolveConfig(site, policy)
	if !reflect.DeepEqual(resolved.Thresholds, customThresholds) {
		t.Fatalf("custom thresholds should win: %#v", resolved.Thresholds)
	}

	resolved.Thresholds[0].MaxRequests = 999
	if customThresholds[0].MaxRequests == 999 {
		t.Fatal("resolved thresholds should not share threshold objects with site")
	}
}

func TestResolveConfigDefaultThresholdFallback(t *testing.T) {
	for _, policy := range []*nodeconfigs.HTTPCCPolicy{
		nil,
		{},
	} {
		site := &serverconfigs.HTTPCCConfig{UseDefaultThresholds: true}
		resolved := ResolveConfig(site, policy)
		if !reflect.DeepEqual(resolved.Thresholds, serverconfigs.DefaultHTTPCCThresholds) {
			t.Fatalf("should fallback to default thresholds: %#v", resolved.Thresholds)
		}

		resolved.Thresholds[0].MaxRequests = 999
		if serverconfigs.DefaultHTTPCCThresholds[0].MaxRequests == 999 {
			t.Fatal("fallback thresholds should be cloned")
		}
	}
}

func TestResolveConfigKeepsEmptyCustomThresholds(t *testing.T) {
	site := &serverconfigs.HTTPCCConfig{
		UseDefaultThresholds: false,
		Thresholds:           nil,
	}
	policy := nodeconfigs.NewHTTPCCPolicy()

	resolved := ResolveConfig(site, policy)
	if len(resolved.Thresholds) != 0 {
		t.Fatalf("empty custom thresholds should stay empty: %#v", resolved.Thresholds)
	}
}

func TestResolveConfigDoesNotMutateSite(t *testing.T) {
	site := &serverconfigs.HTTPCCConfig{
		UseDefaultThresholds: true,
		Thresholds: []*serverconfigs.HTTPCCThreshold{{
			PeriodSeconds: 1,
			MaxRequests:   2,
			BlockSeconds:  3,
		}},
		Action: "",
	}
	originalThresholds := site.Thresholds

	resolved := ResolveConfig(site, nodeconfigs.NewHTTPCCPolicy())
	if resolved == site {
		t.Fatal("resolved config should be a new object")
	}
	if site.Action != "" {
		t.Fatalf("site action should remain unchanged: %q", site.Action)
	}
	if site.Thresholds[0] != originalThresholds[0] {
		t.Fatal("site thresholds should remain untouched")
	}
}

func TestResolveConfigNil(t *testing.T) {
	if ResolveConfig(nil, nil) != nil {
		t.Fatal("nil site should resolve to nil")
	}
}
