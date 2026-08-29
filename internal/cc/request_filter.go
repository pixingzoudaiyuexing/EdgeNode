package cc

import (
	"path/filepath"

	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/utils"
)

// MatchURL 判断当前 URL 是否进入 CC 统计。
// OnlyURLPatterns / ExceptURLPatterns 的具体匹配语义由 EdgeCommon 的 1.3.9
// 兼容配置模型统一负责，节点运行时不再重复实现一套通配规则。
func MatchURL(config *serverconfigs.HTTPCCConfig, requestURL string) bool {
	if config == nil {
		return false
	}
	return config.MatchURL(requestURL)
}

// ShouldIgnoreCommonFile 判断请求是否应按“忽略常见文件”设置跳过 CC 统计。
//
// 精确的 1.3.9 WAF cc2 实现并不是看到静态扩展名就直接跳过，而是要求请求
// 同时带有 Referer，随后才检查 URL Path 的扩展名。高级 CC 先复用这一已确认
// 的同代行为，避免把浏览器直接访问静态文件和页面内资源请求混为一谈。
func ShouldIgnoreCommonFile(config *serverconfigs.HTTPCCConfig, requestPath string, referer string) bool {
	if config == nil || !config.IgnoreCommonFiles || referer == "" {
		return false
	}

	ext := filepath.Ext(requestPath)
	return ext != "" && utils.IsCommonFileExtension(ext)
}
