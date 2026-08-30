//go:build plus

package nodes

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	iplib "github.com/TeaOSLab/EdgeCommon/pkg/iplibrary"
	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	edgecc "github.com/TeaOSLab/EdgeNode/internal/cc"
	"github.com/TeaOSLab/EdgeNode/internal/utils/agents"
	"github.com/TeaOSLab/EdgeNode/internal/utils/counters"
	"github.com/TeaOSLab/EdgeNode/internal/utils/fasttime"
	"github.com/TeaOSLab/EdgeNode/internal/utils/ttlcache"
)

const (
	ccGET302PermissionPrefix = "HTTP-CC-GET302-"
	ccGET302RedirectSuffix   = "-REDIRECTS"
	ccGET302FreshSeconds     = int64(10)
	ccGET302ReplaySeconds    = int64(300)
	ccGET302PermissionLife   = int64(600)
)

// doCCGET302 执行 1.3.9 Plus 的 GET302 浏览器校验。
//
// 已通过可信 1.3.9 Plus 二进制静态审计确认：
//   - 只处理 GET 请求；
//   - 客户端 Agent IP、百度/Google 验证路径和搜索引擎线路直接跳过；
//   - 校验地址默认 /GE/CC/VALIDATOR；
//   - key = MD5(secret@url@ip).MD5(secret@timestamp).timestamp；
//   - 新鲜回调（now-timestamp<=10）获得 600 秒本机临时放行；
//   - 10~300 秒的回放返回 403；超过 300 秒只跳回原 URL，不授予放行；
//   - 发起挑战和处理回调前都会检查连续无效跳转次数。
//
// 返回 true 表示当前请求已经由 GET302 分支处理（302、403 或连续跳转封禁）；
// 返回 false 表示继续执行后续普通 CC 请求阈值。
func (this *HTTPRequest) doCCGET302(config *serverconfigs.HTTPCCConfig, policy *nodeconfigs.HTTPCCPolicy, remoteAddr string, firewallScope firewallconfigs.FirewallScope) bool {
	if this == nil || this.RawReq == nil || this.writer == nil || config == nil || !config.EnableGET302 {
		return false
	}
	if this.RawReq.Method != http.MethodGet || len(remoteAddr) == 0 {
		return false
	}

	// 原版会放过已识别的客户端 Agent，避免客户端程序被浏览器重定向协议影响。
	if agents.SharedManager.ContainsIP(remoteAddr) {
		return false
	}

	requestPath := this.RawReq.URL.Path
	// 百度、Google 的站点验证文件需要保持原样可访问。
	if strings.HasPrefix(requestPath, "/baidu_verify_") || strings.HasPrefix(requestPath, "/google") {
		return false
	}

	// 原版还会根据 IP 库中的运营商/线路名称识别搜索引擎来源。
	lookupResult := iplib.LookupIP(remoteAddr)
	if lookupResult != nil && lookupResult.IsOk() && ccGET302IsSearchProvider(lookupResult.ProviderName()) {
		return false
	}

	validatePath, _, _, _ := ccGET302RedirectConfig(policy)
	permissionKey := ccGET302PermissionKey(remoteAddr)

	// Validator 回调必须优先于临时放行缓存处理；可信原版也是先识别回调，
	// 再检查普通请求是否已有 600 秒临时权限。
	if requestPath == validatePath {
		if !this.checkCCRedirects(policy, remoteAddr, firewallScope) {
			return true
		}
		return this.handleCCGET302Callback(permissionKey)
	}

	if ttlcache.SharedInt64Cache.Read(permissionKey) != nil {
		return false
	}

	if !this.checkCCRedirects(policy, remoteAddr, firewallScope) {
		return true
	}

	fullURL := this.ccGET302FullURL()
	timestamp := fasttime.Now().Unix()
	key := ccGET302Key(this.ccGET302NodeSecret(), fullURL, remoteAddr, timestamp)
	target := validatePath + "?key=" + key + "&url=" + url.QueryEscape(fullURL)
	httpRedirect(this.writer, this.RawReq, target, http.StatusFound)
	return true
}

// handleCCGET302Callback 校验 /GE/CC/VALIDATOR 回调并执行原版时间窗口行为。
func (this *HTTPRequest) handleCCGET302Callback(permissionKey string) bool {
	query := this.RawReq.URL.Query()
	keyParts := strings.Split(query.Get("key"), ".")
	if len(keyParts) != 3 {
		this.writeCode(http.StatusForbidden, ccConnections403EN, ccConnections403ZHCN)
		return true
	}

	originalURL := query.Get("url")
	secret := this.ccGET302NodeSecret()
	remoteAddr := this.requestRemoteAddr(true)

	if keyParts[0] != ccGET302MD5(secret+"@"+originalURL+"@"+remoteAddr) {
		this.writeCode(http.StatusForbidden, ccConnections403EN, ccConnections403ZHCN)
		return true
	}
	if keyParts[1] != ccGET302MD5(secret+"@"+keyParts[2]) {
		this.writeCode(http.StatusForbidden, ccConnections403EN, ccConnections403ZHCN)
		return true
	}

	// TeaGo types.Int64() 对无效数字返回 0；这里显式保持同一结果。
	timestamp, _ := strconv.ParseInt(keyParts[2], 10, 64)
	now := fasttime.Now().Unix()
	delta := now - timestamp

	if delta <= ccGET302FreshSeconds {
		// 原版没有限制负 delta；未来时间戳同样进入此分支，因此这里不额外“修正”。
		ttlcache.SharedInt64Cache.Write(permissionKey, 1, now+ccGET302PermissionLife)
		httpRedirect(this.writer, this.RawReq, originalURL, http.StatusFound)
		return true
	}

	if delta <= ccGET302ReplaySeconds {
		this.writeCode(http.StatusForbidden, ccConnections403EN, ccConnections403ZHCN)
		return true
	}

	// 很旧的 Validator URL 不再视为攻击，只送回原地址；由于没有写入临时权限，
	// 浏览器回到原地址后仍会重新接受新的 GET302 校验。
	httpRedirect(this.writer, this.RawReq, originalURL, http.StatusFound)
	return true
}

// checkCCRedirects 限制同一 IP 在短时间内反复进入 GET302 校验的次数。
// 返回 false 表示已经按策略写入临时黑名单并终止连接。
func (this *HTTPRequest) checkCCRedirects(policy *nodeconfigs.HTTPCCPolicy, remoteAddr string, firewallScope firewallconfigs.FirewallScope) bool {
	if this == nil || this.ReqServer == nil || this.ReqServer.Id <= 0 || len(remoteAddr) == 0 {
		return true
	}

	_, durationSeconds, maxRedirects, blockSeconds := ccGET302RedirectConfig(policy)
	count := counters.SharedCounter.IncreaseKey(ccGET302RedirectCounterKey(remoteAddr), durationSeconds)
	if count < uint32(maxRedirects) {
		return true
	}

	reason := fmt.Sprintf("CC防护拦截：在%d秒内无效跳转%d次", durationSeconds, maxRedirects)
	edgecc.RecordBlockedIP(
		this.ReqServer.Id,
		remoteAddr,
		firewallScope,
		blockSeconds,
		firewallScope == firewallconfigs.FirewallScopeGlobal,
		reason,
	)
	// 可信原版 checkCCRedirects() 在 RecordIP() 后仍显式关闭当前请求连接。
	this.Close()
	return false
}

func ccGET302RedirectConfig(policy *nodeconfigs.HTTPCCPolicy) (validatePath string, durationSeconds int, maxRedirects int, blockSeconds int) {
	validatePath = edgecc.RedirectsValidatePath
	durationSeconds = edgecc.RedirectsDurationSeconds
	maxRedirects = edgecc.RedirectsMaxRedirects
	blockSeconds = edgecc.RedirectsBlockSeconds
	if policy == nil {
		return
	}

	redirects := policy.RedirectsChecking
	if len(redirects.ValidatePath) > 0 {
		validatePath = redirects.ValidatePath
	}
	if redirects.DurationSeconds > 0 {
		durationSeconds = redirects.DurationSeconds
	}
	if redirects.MaxRedirects > 0 {
		maxRedirects = redirects.MaxRedirects
	}
	if redirects.BlockSeconds > 0 {
		blockSeconds = redirects.BlockSeconds
	}
	return
}

func ccGET302PermissionKey(remoteAddr string) string {
	return ccGET302PermissionPrefix + remoteAddr
}

func ccGET302RedirectCounterKey(remoteAddr string) string {
	return ccGET302PermissionPrefix + remoteAddr + ccGET302RedirectSuffix
}

func ccGET302IsSearchProvider(providerName string) bool {
	return strings.Contains(providerName, "百度") ||
		strings.Contains(providerName, "谷歌") ||
		strings.Contains(providerName, "baidu") ||
		strings.Contains(providerName, "google")
}

func ccGET302Key(secret string, fullURL string, remoteAddr string, timestamp int64) string {
	timestampString := strconv.FormatInt(timestamp, 10)
	return ccGET302MD5(secret+"@"+fullURL+"@"+remoteAddr) + "." +
		ccGET302MD5(secret+"@"+timestampString) + "." + timestampString
}

func ccGET302MD5(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (this *HTTPRequest) ccGET302FullURL() string {
	scheme := "http"
	if this.IsHTTPS {
		scheme = "https"
	}
	return scheme + "://" + this.ReqHost + this.uri
}

func (this *HTTPRequest) ccGET302NodeSecret() string {
	// 1.3.9 Plus 直接读取进程当前 sharedNodeConfig.Secret。
	if sharedNodeConfig != nil {
		return sharedNodeConfig.Secret
	}
	// 单元测试或极早期初始化阶段没有全局快照时，退回当前请求的节点快照；
	// 正常节点运行路径不会进入这个分支。
	if this != nil && this.nodeConfig != nil {
		return this.nodeConfig.Secret
	}
	return ""
}
