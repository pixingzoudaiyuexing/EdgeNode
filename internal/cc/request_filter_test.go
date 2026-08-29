package cc

import (
	"testing"

	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/shared"
)

func TestMatchURL(t *testing.T) {
	config := &serverconfigs.HTTPCCConfig{
		OnlyURLPatterns: []*shared.URLPattern{
			{Type: shared.URLPatternTypeWildcard, Pattern: "/search*"},
		},
		ExceptURLPatterns: []*shared.URLPattern{
			{Type: shared.URLPatternTypeWildcard, Pattern: "/search/api*"},
		},
	}
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}

	if !MatchURL(config, "https://example.com/search?q=test") {
		t.Fatal("限制 URL 应进入 CC")
	}
	if MatchURL(config, "https://example.com/search/api/list") {
		t.Fatal("例外 URL 应跳过 CC")
	}
	if MatchURL(config, "https://example.com/index") {
		t.Fatal("不匹配限制 URL 的请求应跳过 CC")
	}
	if MatchURL(nil, "https://example.com/search") {
		t.Fatal("nil 配置不应进入 CC")
	}
}

func TestShouldIgnoreCommonFile(t *testing.T) {
	config := &serverconfigs.HTTPCCConfig{IgnoreCommonFiles: true}

	cases := []struct {
		name    string
		path    string
		referer string
		want    bool
	}{
		{name: "浏览器页面资源", path: "/assets/app.js", referer: "https://example.com/", want: true},
		{name: "大小写扩展名", path: "/assets/IMAGE.PNG", referer: "https://example.com/", want: true},
		{name: "直接访问静态文件", path: "/assets/app.js", referer: "", want: false},
		{name: "动态页面", path: "/api/list", referer: "https://example.com/", want: false},
		{name: "未知扩展名", path: "/download/file.zip", referer: "https://example.com/", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ShouldIgnoreCommonFile(config, c.path, c.referer); got != c.want {
				t.Fatalf("ShouldIgnoreCommonFile() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestShouldIgnoreCommonFileDisabled(t *testing.T) {
	config := &serverconfigs.HTTPCCConfig{IgnoreCommonFiles: false}
	if ShouldIgnoreCommonFile(config, "/assets/app.js", "https://example.com/") {
		t.Fatal("关闭 IgnoreCommonFiles 后不应跳过")
	}
	if ShouldIgnoreCommonFile(nil, "/assets/app.js", "https://example.com/") {
		t.Fatal("nil 配置不应跳过")
	}
}
