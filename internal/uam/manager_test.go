package uam

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestLoadPageToFinalKeyFlow(t *testing.T) {
	manager, err := NewManager("test-node-id", "test-node-secret")
	if err != nil {
		t.Fatal(err)
	}

	const remoteAddr = "203.0.113.9"
	const userAgent = "Mozilla/5.0 UAM-Full-Flow"

	// 第一步：通过真实 LoadPage() 生成 ge_ua_p 和页面 nonce，而不是测试里自行构造预 Key。
	pageReq := httptest.NewRequest(http.MethodGet, "https://www.example.com/path", nil)
	pageReq.Header.Set("User-Agent", userAgent)
	pageRecorder := httptest.NewRecorder()
	if err := manager.LoadPage(pageRecorder, pageReq, remoteAddr, PageOptions{
		IncludeSubdomains: true,
	}); err != nil {
		t.Fatal(err)
	}

	pageResult := pageRecorder.Result()
	defer pageResult.Body.Close()

	var preCookie *http.Cookie
	for _, cookie := range pageResult.Cookies() {
		if cookie.Name == CookiePreKey {
			preCookie = cookie
			break
		}
	}
	if preCookie == nil || preCookie.Value == "" {
		t.Fatal("LoadPage should set preliminary UAM cookie")
	}

	body := pageRecorder.Body.String()
	nonceStart := strings.Index(body, "nonce=")
	if nonceStart < 0 {
		t.Fatal("challenge page should contain nonce")
	}
	nonceStart += len("nonce=")
	nonceEnd := strings.IndexByte(body[nonceStart:], ',')
	if nonceEnd < 0 {
		t.Fatal("challenge page nonce should be followed by comma")
	}
	nonce, err := strconv.Atoi(body[nonceStart : nonceStart+nonceEnd])
	if err != nil || nonce < 1000 || nonce > 9999 {
		t.Fatalf("unexpected challenge nonce: %q", body[nonceStart:nonceStart+nonceEnd])
	}

	// 第二步：模拟浏览器脚本使用实际 ge_ua_p 计算 sum，并提交 Challenge POST。
	form := url.Values{}
	form.Set("nonce", strconv.Itoa(nonce))
	form.Set("sum", strconv.FormatInt(challengeSum(preCookie.Value, nonce), 10))
	postReq := httptest.NewRequest(http.MethodPost, "https://api.example.com/path", strings.NewReader(form.Encode()))
	postReq.Header.Set("User-Agent", userAgent)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set(StepHeader, StepPrevious)
	postReq.AddCookie(preCookie)

	postRecorder := httptest.NewRecorder()
	if err := manager.CheckPrevKey(postRecorder, postReq, remoteAddr, CheckOptions{
		KeyLife:           3600,
		IncludeSubdomains: true,
	}); err != nil {
		t.Fatal(err)
	}
	if body := postRecorder.Body.String(); body != "{\"ok\":true}" {
		t.Fatalf("unexpected challenge result: %q", body)
	}

	var finalCookie *http.Cookie
	for _, cookie := range postRecorder.Result().Cookies() {
		if cookie.Name == CookieKey {
			finalCookie = cookie
			break
		}
	}
	if finalCookie == nil || finalCookie.Value == "" {
		t.Fatal("successful challenge should set final UAM cookie")
	}

	// 第三步：最终 Cookie 应能在同一可注册主域的另一个子域通过校验。
	checkReq := httptest.NewRequest(http.MethodGet, "https://cdn.example.com/resource", nil)
	checkReq.Header.Set("User-Agent", userAgent)
	checkReq.AddCookie(finalCookie)
	if err := manager.CheckKey(checkReq, remoteAddr, CheckOptions{
		KeyLife:           3600,
		IncludeSubdomains: true,
	}); err != nil {
		t.Fatalf("final UAM cookie should validate after real LoadPage flow: %v", err)
	}
}

func TestCheckPrevKeyReturnsCompatibleJSONAndFinalCookie(t *testing.T) {
	manager, err := NewManager("test-node-id", "test-node-secret")
	if err != nil {
		t.Fatal(err)
	}

	const remoteAddr = "203.0.113.10"
	const userAgent = "Mozilla/5.0 UAM-Test"
	const nonce = 4321

	seedReq := httptest.NewRequest(http.MethodGet, "https://www.example.com/path", nil)
	seedReq.Header.Set("User-Agent", userAgent)
	encoded, err := manager.EncodeKey(manager.ComposeKey(seedReq, remoteAddr))
	if err != nil {
		t.Fatal(err)
	}
	preCookieValue := url.QueryEscape(encoded)

	form := url.Values{}
	form.Set("nonce", strconv.Itoa(nonce))
	form.Set("sum", strconv.FormatInt(challengeSum(preCookieValue, nonce), 10))
	postReq := httptest.NewRequest(http.MethodPost, "https://www.example.com/path", strings.NewReader(form.Encode()))
	postReq.Header.Set("User-Agent", userAgent)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: CookiePreKey, Value: preCookieValue})

	recorder := httptest.NewRecorder()
	err = manager.CheckPrevKey(recorder, postReq, remoteAddr, CheckOptions{
		KeyLife:           3600,
		IncludeSubdomains: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := recorder.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.StatusCode)
	}
	if contentType := result.Header.Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	if body := recorder.Body.String(); body != "{\"ok\":true}" {
		t.Fatalf("unexpected body: %q", body)
	}

	var finalCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == CookieKey {
			finalCookie = cookie
			break
		}
	}
	if finalCookie == nil || finalCookie.Value == "" {
		t.Fatal("missing final UAM cookie")
	}

	checkReq := httptest.NewRequest(http.MethodGet, "https://api.example.com/path", nil)
	checkReq.Header.Set("User-Agent", userAgent)
	checkReq.AddCookie(finalCookie)
	if err := manager.CheckKey(checkReq, remoteAddr, CheckOptions{
		KeyLife:           3600,
		IncludeSubdomains: true,
	}); err != nil {
		t.Fatalf("final UAM cookie should validate: %v", err)
	}
}

func TestCheckPrevKeyRejectsWrongSum(t *testing.T) {
	manager, err := NewManager("test-node-id", "test-node-secret")
	if err != nil {
		t.Fatal(err)
	}

	const remoteAddr = "203.0.113.11"
	seedReq := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	seedReq.Header.Set("User-Agent", "Mozilla/5.0 UAM-Test")
	encoded, err := manager.EncodeKey(manager.ComposeKey(seedReq, remoteAddr))
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{"nonce": {"1234"}, "sum": {"1"}}
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", strings.NewReader(form.Encode()))
	postReq.Header.Set("User-Agent", seedReq.UserAgent())
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: CookiePreKey, Value: url.QueryEscape(encoded)})

	recorder := httptest.NewRecorder()
	if err := manager.CheckPrevKey(recorder, postReq, remoteAddr, CheckOptions{}); err == nil {
		t.Fatal("wrong challenge sum should be rejected")
	}
}

func TestLoadPageRequiresSuccessfulJSONResult(t *testing.T) {
	manager, err := NewManager("test-node-id", "test-node-secret")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://www.example.com/path", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 UAM-Test")
	recorder := httptest.NewRecorder()
	if err := manager.LoadPage(recorder, req, "203.0.113.12", PageOptions{}); err != nil {
		t.Fatal(err)
	}

	body := recorder.Body.String()
	for _, expected := range []string{
		"JSON.parse(xhr.responseText||'{}')",
		"response.ok!==true",
		"xhr.onerror=retry",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("challenge page should contain %q", expected)
		}
	}
}
