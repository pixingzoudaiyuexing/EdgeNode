//go:build plus

package nodes

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TeaOSLab/EdgeCommon/pkg/iplibrary"
	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/remotelogs"
	"github.com/TeaOSLab/EdgeNode/internal/uam"
	"github.com/TeaOSLab/EdgeNode/internal/utils/ttlcache"
	wafutils "github.com/TeaOSLab/EdgeNode/internal/waf/utils"
)

var sharedUAMQPSTracker = uam.NewQPSTracker()

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
// 当前阶段恢复请求触发、Key 校验和浏览器 Challenge；失败计数及防火墙封禁在 UAM-3 单独恢复。
func (this *HTTPRequest) doUAM() (block bool) {
	if this == nil || this.RawReq == nil || this.web == nil || this.web.UAM == nil || !this.web.UAM.IsOn {
		return false
	}

	remoteAddr := this.requestRemoteAddr(true)
	policy := this.findUAMPolicy()

	// Challenge POST 必须被当前请求直接消费，不能在失败后继续进入源站。
	// 即使策略刚好在页面展示后被动态关闭，也仍然完成/拒绝这次协议回调。
	if this.isUAMRequest() {
		return this.doUAMChallenge(remoteAddr, policy)
	}

	if policy == nil || !policy.IsOn {
		return false
	}

	config := this.web.UAM

	// 已经通过当前节点 UAM 校验并被临时记忆的 IP，仅跳过 UAM，不跳过 WAF。
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
		this.writeCode(http.StatusForbidden, "Spider is not allowed.", "当前客户端不允许访问")
		return true
	}

	if !sharedUAMQPSTracker.Triggered(this.uamQPSKey(remoteAddr), config.MinQPSPerIP) {
		return false
	}

	manager, err := this.newUAMManager()
	if err != nil {
		remotelogs.Error("UAM", "initialize UAM manager failed: "+err.Error())
		this.writeCode(http.StatusInternalServerError, "Failed to initialize UAM.", "5秒盾初始化失败")
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
		remotelogs.Error("UAM", "load UAM page failed: "+err.Error())
		this.writeCode(http.StatusInternalServerError, "Failed to load UAM page.", "5秒盾页面加载失败")
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
		// 对外保持固定 JSON，不暴露 Key、加密或校验失败的内部细节。
		remotelogs.Warn("UAM", "UAM challenge failed: "+err.Error())
		this.writeUAMResult(false)
	}
	return true
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

func (this *HTTPRequest) uamAllowedKey(remoteAddr string) string {
	return "uam:allowed:" + this.uamQPSKey(remoteAddr)
}

func (this *HTTPRequest) isUAMTemporarilyAllowed(remoteAddr string) bool {
	if remoteAddr == "" {
		return false
	}
	return ttlcache.SharedInt64Cache.Read(this.uamAllowedKey(remoteAddr)) != nil
}

func (this *HTTPRequest) rememberUAMAllowedIP(remoteAddr string, keyLife int) {
	if remoteAddr == "" {
		return
	}
	if keyLife <= 0 {
		keyLife = 3600
	}
	ttlcache.SharedInt64Cache.Write(this.uamAllowedKey(remoteAddr), 1, time.Now().Unix()+int64(keyLife))
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
