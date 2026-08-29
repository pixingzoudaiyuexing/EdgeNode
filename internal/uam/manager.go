package uam

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TeaOSLab/EdgeNode/internal/utils/encrypt"
	"golang.org/x/net/publicsuffix"
)

const (
	CookieKey     = "ge_ua_key"
	CookiePreKey  = "ge_ua_p"
	StepHeader    = "X-GE-UA-Step"
	StepPrevious  = "prev"
	preKeySeconds = 5
)

// CheckOptions 是最终 UAM Key 的校验参数。
type CheckOptions struct {
	KeyLife           int
	IncludeSubdomains bool
}

// PageOptions 是浏览器挑战页面的可配置文本。
type PageOptions struct {
	Title             string
	Body              string
	IncludeSubdomains bool
}

// Manager 实现与 1.3.9 Plus 外部协议兼容的 UAM Key 和浏览器挑战。
// 加密材料由节点自身的 NodeId/Secret 提供，不依赖任何外部授权或官方服务。
type Manager struct {
	method encrypt.MethodInterface
}

func NewManager(key, iv string) (*Manager, error) {
	method, err := encrypt.NewMethodInstance("aes-256-cfb", key, iv)
	if err != nil {
		return nil, err
	}
	return &Manager{method: method}, nil
}

func (m *Manager) ComposeKey(req *http.Request, remoteAddr string) *Key {
	key := &Key{
		Timestamp: time.Now().Unix(),
		Version:   keyVersion,
		Host:      requestHost(req),
	}
	key.Put(remoteAddr, req.UserAgent())
	return key
}

func (m *Manager) EncodeKey(key *Key) (string, error) {
	if m == nil || m.method == nil {
		return "", errors.New("uam manager is not initialized")
	}
	if key == nil {
		return "", errors.New("uam key is nil")
	}

	ciphertext, err := m.method.Encrypt(key.marshal())
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (m *Manager) decodeKey(encoded string) (*Key, error) {
	if m == nil || m.method == nil {
		return nil, errors.New("uam manager is not initialized")
	}

	decodedValue, err := url.QueryUnescape(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode key failed: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(decodedValue)
	if err != nil {
		return nil, fmt.Errorf("decode key failed: %w", err)
	}
	plaintext, err := m.method.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt key failed: %w", err)
	}
	key, err := unmarshalKey(plaintext)
	if err != nil {
		return nil, fmt.Errorf("unmarshal key failed: %w", err)
	}
	return key, nil
}

// CheckKey 校验浏览器最终 Key。
func (m *Manager) CheckKey(req *http.Request, remoteAddr string, options CheckOptions) error {
	cookie, err := req.Cookie(CookieKey)
	if err != nil {
		return fmt.Errorf("read cookie failed: %w", err)
	}
	key, err := m.decodeKey(cookie.Value)
	if err != nil {
		return err
	}

	life := options.KeyLife
	if life <= 0 {
		life = 3600
	}
	if err := validateKey(key, req, remoteAddr, life, options.IncludeSubdomains); err != nil {
		return err
	}
	return nil
}

// CheckPrevKey 校验浏览器第一阶段 POST，并写入最终 ge_ua_key。
// 返回 nil 表示挑战已经通过。
func (m *Manager) CheckPrevKey(writer http.ResponseWriter, req *http.Request, remoteAddr string, options CheckOptions) error {
	cookie, err := req.Cookie(CookiePreKey)
	if err != nil {
		return fmt.Errorf("read cookie failed: %w", err)
	}
	prevKey, err := m.decodeKey(cookie.Value)
	if err != nil {
		return err
	}
	if err := validateKey(prevKey, req, remoteAddr, 30, options.IncludeSubdomains); err != nil {
		return err
	}

	if err := req.ParseForm(); err != nil {
		return fmt.Errorf("parse uam form failed: %w", err)
	}
	nonce, err := strconv.Atoi(req.Form.Get("nonce"))
	if err != nil || nonce <= 0 {
		return errors.New("invalid uam nonce")
	}
	sum, err := strconv.ParseInt(req.Form.Get("sum"), 10, 64)
	if err != nil {
		return errors.New("invalid uam sum")
	}
	if challengeSum(cookie.Value, nonce) != sum {
		return errors.New("verify uam sum failed")
	}

	life := options.KeyLife
	if life <= 0 {
		life = 3600
	}
	finalKey := m.ComposeKey(req, remoteAddr)
	encoded, err := m.EncodeKey(finalKey)
	if err != nil {
		return err
	}

	finalCookie := &http.Cookie{
		Name:     CookieKey,
		Value:    url.QueryEscape(encoded),
		Path:     "/",
		Expires:  time.Now().Add(time.Duration(life) * time.Second),
		MaxAge:   life,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if options.IncludeSubdomains {
		finalCookie.Domain = cookieDomain(requestHost(req))
	}
	http.SetCookie(writer, finalCookie)
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, err = writer.Write([]byte(`{"ok":true}`))
	return err
}

// LoadPage 下发第一阶段 ge_ua_p 并输出浏览器 JavaScript 挑战页面。
func (m *Manager) LoadPage(writer http.ResponseWriter, req *http.Request, remoteAddr string, options PageOptions) error {
	key := m.ComposeKey(req, remoteAddr)
	encoded, err := m.EncodeKey(key)
	if err != nil {
		return err
	}

	preCookie := &http.Cookie{
		Name:     CookiePreKey,
		Value:    url.QueryEscape(encoded),
		Path:     "/",
		Expires:  time.Now().Add(preKeySeconds * time.Second),
		MaxAge:   preKeySeconds,
		HttpOnly: false, // 浏览器脚本需要读取此值计算 sum。
		SameSite: http.SameSiteLaxMode,
	}
	if options.IncludeSubdomains {
		preCookie.Domain = cookieDomain(requestHost(req))
	}
	http.SetCookie(writer, preCookie)

	nonce, err := randomNonce()
	if err != nil {
		return err
	}

	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = "正在检查您的浏览器"
	}
	body := strings.TrimSpace(options.Body)
	if body == "" {
		body = "安全检查通过后页面会自动刷新，请稍候。"
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.WriteHeader(http.StatusOK)

	_, err = fmt.Fprintf(writer, `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>body{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:0;background:#f6f7f9;color:#222}.box{max-width:620px;margin:12vh auto;padding:32px;background:#fff;border-radius:12px;box-shadow:0 8px 30px rgba(0,0,0,.08)}h1{font-size:22px;margin:0 0 16px}p{line-height:1.7}</style>
<script>
(function(){
  var cpk=%q, nonce=%d, step=%q;
  function cookieValue(name){
    var parts=document.cookie.split(';');
    for(var i=0;i<parts.length;i++){
      var p=parts[i].trim(), x=p.indexOf('=');
      if(x>0 && p.substring(0,x)===name) return p.substring(x+1);
    }
    return '';
  }
  function run(){
    var value=cookieValue(cpk);
    if(!value){ window.setTimeout(function(){location.reload();},1000); return; }
    var sum=0;
    for(var i=0;i<value.length;i++){
      var c=value.charAt(i);
      if(/^[a-zA-Z0-9]$/.test(c)) sum+=value.charCodeAt(i)*(nonce+i);
    }
    var xhr=new XMLHttpRequest();
    xhr.open('POST',window.location.toString(),true);
    xhr.setRequestHeader('Content-type','application/x-www-form-urlencoded');
    xhr.setRequestHeader('X-GE-UA-Step',step);
    xhr.onreadystatechange=function(){
      if(xhr.readyState===4 && xhr.status===200){
        var left=5;
        window.setInterval(function(){
          var el=document.getElementById('counter'); if(el) el.textContent=left; if(left>0) left--;
        },1000);
        window.setTimeout(function(){location.reload();},5000);
      }
    };
    xhr.send('sum='+encodeURIComponent(sum)+'&nonce='+encodeURIComponent(nonce));
  }
  if(window.addEventListener) window.addEventListener('load',run); else window.onload=run;
})();
</script>
</head>
<body><div class="box"><h1>%s</h1><p>%s</p><p>预计还需 <span id="counter">5</span> 秒。</p></div></body>
</html>`, html.EscapeString(title), CookiePreKey, nonce, StepPrevious, html.EscapeString(title), html.EscapeString(body))
	return err
}

func validateKey(key *Key, req *http.Request, remoteAddr string, keyLife int, includeSubdomains bool) error {
	if key == nil || key.Version <= 0 || key.Timestamp <= 0 {
		return errors.New("verify key failed")
	}
	now := time.Now().Unix()
	if key.Timestamp > now+30 || now-key.Timestamp > int64(keyLife) {
		return errors.New("verify key failed")
	}
	if !key.IsSame(remoteAddr, req.UserAgent()) {
		return errors.New("verify key hash failed")
	}

	currentHost := requestHost(req)
	if includeSubdomains {
		if ParseTopDomain(key.Host) != ParseTopDomain(currentHost) {
			return errors.New("verify key domain failed")
		}
	} else if !strings.EqualFold(key.Host, currentHost) {
		return errors.New("verify key domain failed")
	}
	return nil
}

func challengeSum(value string, nonce int) int64 {
	var sum int64
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			sum += int64(c) * int64(nonce+i)
		}
	}
	return sum
}

func randomNonce() (int, error) {
	// 与 1.3.9 可观察行为保持相同范围：1000..9999。
	n, err := rand.Int(rand.Reader, big.NewInt(9000))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + 1000, nil
}

func requestHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	host := req.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.Trim(strings.ToLower(host), "[]")
}

// ParseTopDomain 返回用于 IncludeSubdomains 校验的可注册主域名。
func ParseTopDomain(host string) string {
	host = strings.Trim(strings.ToLower(host), ".[]")
	if host == "" || net.ParseIP(host) != nil || host == "localhost" {
		return host
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return domain
}

func cookieDomain(host string) string {
	domain := ParseTopDomain(host)
	if domain == "" || domain == "localhost" || net.ParseIP(domain) != nil {
		return ""
	}
	return domain
}
