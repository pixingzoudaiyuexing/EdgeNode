//go:build plus

package nodes

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/iplibrary"
	"github.com/TeaOSLab/EdgeNode/internal/remotelogs"
	"github.com/TeaOSLab/EdgeNode/internal/uam"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
	wafutils "github.com/TeaOSLab/EdgeNode/internal/waf/utils"
)

const uamWhiteListIPType = "uam"

var sharedUAMQPSTracker = uam.NewQPSTracker()
var sharedUAMFailureTracker = uam.NewFailureTracker()

// isUAMRequest 只识别 UAM 浏览器挑战的回调请求。
// 正常页面请求仍然在 WAF 之后调用 doUAM；挑战 POST 则需要在 WAF 之前处理，
// 避免挑战协议自身被 WAF 规则误拦截。
func (this *HTTPRequest) isUAMRequest() bool {
	if this == nil || this.RawReq == nil {
		return false
	}
	if this.RawReq.Method != http.MethodPost {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(this.RawReq.Header.Get(uam.StepHeader)), uam.StepPrevious)
}

// doUAM 执行网站级 5 秒盾。
func (this *HTTPRequest) doUAM() (block bool) {
	if this == nil || this.RawReq == nil || this.web == nil || this.web.UAM == nil || !this.web.UAM.IsOn {
		return false
	}

	remoteAddr := this.requestRemoteAddr(true)
	policy := this.findUAMPolicy()
	if policy == nil || !policy.IsOn {
		return false
	}

	// Challenge POST 由当前请求直接消费，且在 WAF 之前处理，避免协议自身被 WAF 拦截。
	// 集群策略关闭后不再处理旧 Challenge 回调，也不会继续累计失败次数。
	if this.isUAMRequest() {
		return this.doUAMChallenge(remoteAddr, policy)
	}

	config := this.web.UAM

	// 通过 5 秒盾后按网站配置加入节点临时 IP 白名单。
	// 这里只使用独立的 uam 类型，所以仅跳过 UAM，不会把该 IP 变成 WAF 全局白名单。
	if this.isUAMTemporarilyAllowed(remoteAddr) {
		return false
	}

	// 节点自动白名单和系统 IP 白名单均应跳过 5 秒盾。
	if this.nodeConfig != nil && this.nodeConfig.IPIsAutoAllowed(remoteAddr) {
		return false
	}
	if this.ReqServer != nil {
		_, isAllowed, _ := iplibrary.AllowIP(remoteAddr, this.ReqServer.Id)
		if isAllowed {
			return false
		}
	}

	if !config.MatchURL(this.URL()) || !config.MatchRequest(this.Format) {
		return false
	}

	userAgent := this.RawReq.UserAgent()
	if policy.AllowSearchEngines && searchEngineRegex.MatchString(userAgent) && wafutils.CheckSearchEngine(remoteAddr) {
		return false
	}
	if policy.DenySpiders && spiderRegexp.MatchString(userAgent) {
		this.sendUAMPlainResponse(http.StatusForbidden, "Spider is not allowed.\n")
		return true
	}

	if !sharedUAMQPSTracker.Triggered(this.uamQPSKey(remoteAddr), config.MinQPSPerIP) {
		return false
	}

	manager, err := this.newUAMManager()
	if err != nil {
		remotelogs.Error("UAM", "initialize UAM manager failed: "+err.Error())
		this.sendUAMPlainResponse(http.StatusInternalServerError, "Failed to initialize UAM.\n")
		return true
	}

	checkOptions := this.uamCheckOptions(config.KeyLife, policy)
	if err = manager.CheckKey(this.RawReq, remoteAddr, checkOptions); err == nil {
		if config.AddToWhiteList {
			this.rememberUAMAllowedIP(remoteAddr, checkOptions.KeyLife)
		}
		return false
	}

	if err = manager.LoadPage(this.writer, this.RawReq, remoteAddr, uam.PageOptions{
		Title:             policy.UITitle,
		Body:              policy.UIBody,
		IncludeSubdomains: policy.IncludeSubdomains,
	}); err != nil {
		// LoadPage 可能已经写出响应头，因此这里只记录错误，避免二次写状态码破坏响应。
		remotelogs.Error("UAM", "load UAM page failed: "+err.Error())
	}
	return true
}

func (this *HTTPRequest) doUAMChallenge(remoteAddr string, policy *nodeconfigs.UAMPolicy) bool {
	if policy == nil {
		policy = nodeconfigs.DefaultUAMPolicy
	}

	manager, err := this.newUAMManager()
	if err != nil {
		remotelogs.Error("UAM", "initialize UAM manager failed: "+err.Error())
		this.writeUAMResult(false)
		return true
	}

	keyLife := 0
	if this.web != nil && this.web.UAM != nil {
		keyLife = this.web.UAM.KeyLife
	}
	if err = manager.CheckPrevKey(this.writer, this.RawReq, remoteAddr, this.uamCheckOptions(keyLife, policy)); err != nil {
		// 只把真实 Challenge POST 校验失败计入连续失败次数；首次访问没有 Key 不属于失败。
		this.recordUAMChallengeFailure(remoteAddr, policy)

		// 对外保持固定 JSON，不暴露 Key、加密或校验失败的内部细节。
		remotelogs.Warn("UAM", "UAM challenge failed: "+err.Error())
		this.writeUAMResult(false)
		return true
	}

	// “连续失败”在成功通过 Challenge 后立即清零；不额外引入未经 1.3.x 证据确认的时间窗口。
	sharedUAMFailureTracker.Reset(this.uamFailureKey(remoteAddr))
	return true
}

func (this *HTTPRequest) recordUAMChallengeFailure(remoteAddr string, policy *nodeconfigs.UAMPolicy) {
	if remoteAddr == "" || policy == nil || this == nil || this.ReqServer == nil {
		return
	}
	if policy.MaxFails <= 0 || policy.BlockSeconds <= 0 {
		return
	}

	failureKey := this.uamFailureKey(remoteAddr)
	fails := sharedUAMFailureTracker.Increase(failureKey)
	if fails < policy.MaxFails {
		return
	}

	// 历史商业版明确存在“超过 N 次验证不通过自动加入 IP 黑名单”和防火墙拦截范围。
	// 这里复用节点现有临时黑名单；由于尚无证据证明 UAM 会直接调用 OS 本地防火墙，
	// useLocalFirewall 保持 false，避免扩大封禁语义。
	waf.SharedIPBlackList.RecordIP(
		waf.IPTypeAll,
		policy.FirewallScope(),
		this.ReqServer.Id,
		remoteAddr,
		time.Now().Unix()+int64(policy.BlockSeconds),
		0,
		false,
		0,
		0,
		"5秒盾验证连续失败超过"+strconv.Itoa(policy.MaxFails)+"次",
	)

	// 已经转入临时黑名单后清除失败计数，避免直接伪造 Challenge POST 不断延长同一次封禁。
	sharedUAMFailureTracker.Reset(failureKey)
}

func (this *HTTPRequest) findUAMPolicy() *nodeconfigs.UAMPolicy {
	if this == nil || this.nodeConfig == nil || this.ReqServer == nil {
		return nil
	}
	policy := this.nodeConfig.FindUAMPolicyWithClusterId(this.ReqServer.ClusterId)
	if policy == nil {
		return nodeconfigs.DefaultUAMPolicy
	}
	return policy
}

func (this *HTTPRequest) newUAMManager() (*uam.Manager, error) {
	if this == nil || this.nodeConfig == nil {
		return nil, errors.New("node config is not initialized")
	}

	// 优先使用集群共享密钥，保证同集群多节点之间可以校验同一个 ge_ua_key。
	// 老配置没有 ClusterSecret 时，再退回节点身份材料；两种方式都只依赖本地配置。
	material := strings.TrimSpace(this.nodeConfig.ClusterSecret)
	if material == "" {
		material = this.nodeConfig.NodeId + "@" + this.nodeConfig.Secret
	}
	if strings.TrimSpace(material) == "" {
		return nil, errors.New("UAM key material is empty")
	}

	key := sha256.Sum256([]byte("goedge-uam-key@" + material))
	iv := sha256.Sum256([]byte("goedge-uam-iv@" + material))
	return uam.NewManager(string(key[:]), string(iv[:16]))
}

func (this *HTTPRequest) uamCheckOptions(webKeyLife int, policy *nodeconfigs.UAMPolicy) uam.CheckOptions {
	keyLife := webKeyLife
	if keyLife <= 0 && policy != nil {
		keyLife = policy.KeyLife
	}
	if keyLife <= 0 {
		keyLife = 3600
	}

	includeSubdomains := false
	if policy != nil {
		includeSubdomains = policy.IncludeSubdomains
	}
	return uam.CheckOptions{
		KeyLife:           keyLife,
		IncludeSubdomains: includeSubdomains,
	}
}

func (this *HTTPRequest) uamQPSKey(remoteAddr string) string {
	serverId := int64(0)
	if this != nil && this.ReqServer != nil {
		serverId = this.ReqServer.Id
	}
	return strconv.FormatInt(serverId, 10) + "@" + remoteAddr
}

func (this *HTTPRequest) uamFailureKey(remoteAddr string) string {
	return "uam:fail:" + this.uamQPSKey(remoteAddr)
}

func (this *HTTPRequest) isUAMTemporarilyAllowed(remoteAddr string) bool {
	if remoteAddr == "" || this == nil || this.ReqServer == nil {
		return false
	}
	return waf.SharedIPWhiteList.Contains(
		uamWhiteListIPType,
		firewallconfigs.FirewallScopeServer,
		this.ReqServer.Id,
		remoteAddr,
	)
}

func (this *HTTPRequest) rememberUAMAllowedIP(remoteAddr string, keyLife int) {
	if remoteAddr == "" || this == nil || this.ReqServer == nil {
		return
	}
	if keyLife <= 0 {
		keyLife = 3600
	}
	waf.SharedIPWhiteList.Add(
		uamWhiteListIPType,
		firewallconfigs.FirewallScopeServer,
		this.ReqServer.Id,
		remoteAddr,
		time.Now().Unix()+int64(keyLife),
	)
}

func (this *HTTPRequest) writeUAMResult(ok bool) {
	if this == nil || this.writer == nil {
		return
	}
	this.writer.Header().Set("Cache-Control", "no-cache")
	this.writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	this.writer.WriteHeader(http.StatusOK)
	if ok {
		_, _ = this.writer.Write([]byte("{\"ok\":true}"))
	} else {
		_, _ = this.writer.Write([]byte("{\"ok\":false}"))
	}
}

func (this *HTTPRequest) sendUAMPlainResponse(status int, body string) {
	if this == nil || this.writer == nil {
		return
	}
	this.writer.Header().Set("Cache-Control", "no-cache")
	this.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	this.writer.Send(status, body)
}
