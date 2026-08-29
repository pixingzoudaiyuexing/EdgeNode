package uam

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

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
	if body := recorder.Body.String(); body != `{"ok":true}` {
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
