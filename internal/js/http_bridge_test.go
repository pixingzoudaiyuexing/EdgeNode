//go:build script

package js

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/iwind/TeaGo/maps"
)

type bridgeRequestFixture struct {
	header       http.Header
	uri          string
	setAttr      map[string]string
	setVar       map[string]string
	done         bool
	closed       bool
	allowed      bool
	deleted      string
	lastSetName  string
	lastSetValue []string
}

func newBridgeRequestFixture() *bridgeRequestFixture {
	return &bridgeRequestFixture{header: http.Header{"X-Initial": {"one", "two"}}, setAttr: map[string]string{}, setVar: map[string]string{}}
}
func (r *bridgeRequestFixture) Allow()                     { r.allowed = true }
func (r *bridgeRequestFixture) Close()                     { r.closed = true }
func (r *bridgeRequestFixture) ContentLength() int64       { return 123 }
func (r *bridgeRequestFixture) Cookie(name string) string  { return "cookie:" + name }
func (r *bridgeRequestFixture) DeleteHeader(name string)   { r.deleted = name; r.header.Del(name) }
func (r *bridgeRequestFixture) Done()                      { r.done = true }
func (r *bridgeRequestFixture) Format(s string) string     { return "fmt:" + s }
func (r *bridgeRequestFixture) Header() http.Header        { return r.header }
func (r *bridgeRequestFixture) Host() string               { return "example.com" }
func (r *bridgeRequestFixture) Id() string                 { return "request-1" }
func (r *bridgeRequestFixture) Method() string             { return http.MethodPost }
func (r *bridgeRequestFixture) Node() maps.Map             { return maps.Map{"id": int64(22)} }
func (r *bridgeRequestFixture) Path() string               { return "/demo" }
func (r *bridgeRequestFixture) Proto() string              { return "HTTP/2.0" }
func (r *bridgeRequestFixture) ProtoMajor() int            { return 2 }
func (r *bridgeRequestFixture) ProtoMinor() int            { return 0 }
func (r *bridgeRequestFixture) RawRemoteAddr() string      { return "10.0.0.1:12345" }
func (r *bridgeRequestFixture) RemoteAddr() string         { return "10.0.0.1" }
func (r *bridgeRequestFixture) RemotePort() int            { return 12345 }
func (r *bridgeRequestFixture) Server() maps.Map           { return maps.Map{"id": int64(11)} }
func (r *bridgeRequestFixture) SetAttr(name, value string) { r.setAttr[name] = value }
func (r *bridgeRequestFixture) SetHeader(name string, values []string) {
	r.lastSetName, r.lastSetValue = name, append([]string(nil), values...)
	r.header[name] = append([]string(nil), values...)
}
func (r *bridgeRequestFixture) SetURI(uri string)         { r.uri = uri }
func (r *bridgeRequestFixture) SetVar(name, value string) { r.setVar[name] = value }
func (r *bridgeRequestFixture) TransferEncoding() string  { return "chunked" }
func (r *bridgeRequestFixture) URI() string {
	if r.uri == "" {
		return "/demo?a=1"
	}
	return r.uri
}
func (r *bridgeRequestFixture) URL() string { return "https://example.com" + r.URI() }

type bridgeResponseFixture struct {
	header         http.Header
	deleted        string
	sendStatus     int
	sendBody       string
	fileStatus     int
	filePath       string
	respStatus     int
	redirectStatus int
	redirectURL    string
}

func newBridgeResponseFixture() *bridgeResponseFixture {
	return &bridgeResponseFixture{header: http.Header{"X-Resp": {"yes"}}}
}
func (r *bridgeResponseFixture) DeleteHeader(name string) { r.deleted = name; r.header.Del(name) }
func (r *bridgeResponseFixture) Header() http.Header      { return r.header }
func (r *bridgeResponseFixture) Redirect(status int, url string) {
	r.redirectStatus, r.redirectURL = status, url
}
func (r *bridgeResponseFixture) Send(status int, body string) {
	r.sendStatus, r.sendBody = status, body
}
func (r *bridgeResponseFixture) SendFile(status int, path string) (int64, error) {
	r.fileStatus, r.filePath = status, path
	return 456, nil
}
func (r *bridgeResponseFixture) SendResp(resp *http.Response) (int64, error) {
	r.respStatus = resp.StatusCode
	return 789, nil
}
func (r *bridgeResponseFixture) SetHeader(name string, values []string) {
	r.header[name] = append([]string(nil), values...)
}

func newHTTPBridgeContext(t *testing.T) (*Isolate, *Context) {
	t.Helper()
	isolate, err := NewIsolateWithContexts(1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := isolate.GetContext()
	if err != nil {
		isolate.Dispose()
		t.Fatal(err)
	}
	return isolate, ctx
}

func TestHTTPRequestBridgeProtocol(t *testing.T) {
	isolate, ctx := newHTTPBridgeContext(t)
	defer isolate.Dispose()
	req := newBridgeRequestFixture()
	id := ctx.AddGoRequestObject(req)

	value, err := ctx.RunScript(fmt.Sprintf(`
		let req = new gojs.net.http.Request(); req.setGoObject(%d);
		req.uri = "/changed?q=2";
		req.setHeader("X-Array", ["a", 2, null, "b"]);
		req.setHeader("X-Single", 7);
		req.setHeader("X-Null", null);
		req.setAttr("trace", 9);
		req.setVar("X-Via-SetVar", "compat");
		JSON.stringify({
			id:req.id, requestId:req.requestId, url:req.url, path:req.path, uri:req.uri,
			host:req.host, remoteAddr:req.remoteAddr, rawRemoteAddr:req.rawRemoteAddr,
			remotePort:req.remotePort, method:req.method, contentLength:Number(req.contentLength),
			transferEncoding:req.transferEncoding, proto:req.proto, protoMajor:req.protoMajor,
			protoMinor:req.protoMinor, cookie:req.cookie("sid"), formatted:req.format(5),
			server:Number(req.server.id), node:Number(req.node.id), header:req.header["X-Initial"]
		});
	`, id), "http-request-bridge-test.js")
	if err != nil {
		t.Fatal(err)
	}
	got := value.String()
	for _, want := range []string{`"id":"request-1"`, `"uri":"/changed?q=2"`, `"remotePort":12345`, `"cookie":"cookie:sid"`, `"formatted":"fmt:5"`, `"server":11`, `"node":22`} {
		if !strings.Contains(got, want) {
			t.Fatalf("request result %s missing %s", got, want)
		}
	}
	if got := req.header.Values("X-Array"); len(got) != 3 || got[0] != "a" || got[1] != "2" || got[2] != "b" {
		t.Fatalf("array header = %#v", got)
	}
	if got := req.header.Values("X-Single"); len(got) != 1 || got[0] != "7" {
		t.Fatalf("single header = %#v", got)
	}
	if got := req.header.Values("X-Null"); len(got) != 1 || got[0] != "" {
		t.Fatalf("null header = %#v", got)
	}
	if req.setAttr["trace"] != "9" {
		t.Fatalf("attr = %#v", req.setAttr)
	}
	if _, ok := req.setVar["X-Via-SetVar"]; ok {
		t.Fatal("prototype setVar must not call RequestInterface.SetVar in 1.3.9")
	}
	if got := req.header.Get("X-Via-SetVar"); got != "compat" {
		t.Fatalf("setVar compatibility header = %q", got)
	}
}

func TestHTTPResponseBridgeProtocol(t *testing.T) {
	isolate, ctx := newHTTPBridgeContext(t)
	defer isolate.Dispose()
	resp := newBridgeResponseFixture()
	respID := ctx.AddGoResponseObject(resp)
	client := &JSNetHTTPClientLibraryResponse{resp: &http.Response{StatusCode: 418}}
	clientID := ctx.AddGoObject(client)

	_, err := ctx.RunScript(fmt.Sprintf(`
		gojs.prepareNamespace("gojs.net.http.client");
		gojs.net.http.client.Response = class { constructor(id) { this.goObjectId = id } };
		let resp = new gojs.net.http.Response(); resp.setGoObject(%d);
		resp.setHeader("X-Multi", ["a", "b"]);
		resp.send(undefined, 123);
		resp.sendFile(undefined, "/tmp/demo.txt");
		resp.redirect(undefined, "https://example.com/next");
		resp.send(201, new gojs.net.http.client.Response(%d));
	`, respID, clientID), "http-response-bridge-test.js")
	if err != nil {
		t.Fatal(err)
	}
	if resp.sendStatus != 200 || resp.sendBody != "123" {
		t.Fatalf("send = %d %q", resp.sendStatus, resp.sendBody)
	}
	if resp.fileStatus != 200 || resp.filePath != "/tmp/demo.txt" {
		t.Fatalf("sendFile = %d %q", resp.fileStatus, resp.filePath)
	}
	if resp.redirectStatus != 302 || resp.redirectURL != "https://example.com/next" {
		t.Fatalf("redirect = %d %q", resp.redirectStatus, resp.redirectURL)
	}
	if resp.respStatus != 201 || client.resp.StatusCode != 201 {
		t.Fatalf("sendResp status = fixture:%d client:%d", resp.respStatus, client.resp.StatusCode)
	}
	if got := resp.header.Values("X-Multi"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("response header = %#v", got)
	}
}

func TestHTTPBridgeErrors(t *testing.T) {
	isolate, ctx := newHTTPBridgeContext(t)
	defer isolate.Dispose()
	req := newBridgeRequestFixture()
	reqID := ctx.AddGoRequestObject(req)
	resp := newBridgeResponseFixture()
	respID := ctx.AddGoResponseObject(resp)

	_, err := ctx.RunScript(fmt.Sprintf(`let req = new gojs.net.http.Request(); req.setGoObject(%d); req.setHeader("", "x");`, reqID), "empty-header.js")
	if err == nil || !strings.Contains(err.Error(), "header name should not be empty") {
		t.Fatalf("empty header error = %v", err)
	}

	_, err = ctx.RunScript(fmt.Sprintf(`let resp = new gojs.net.http.Response(); resp.setGoObject(%d); resp.redirect(302, "");`, respID), "empty-redirect.js")
	if err == nil || !strings.Contains(err.Error(), "redirect() function require 'url' parameter") {
		t.Fatalf("redirect error = %v", err)
	}

	_, err = ctx.RunScript(`let missingReq = new gojs.net.http.Request(); missingReq.setGoObject(999999); missingReq.id;`, "missing-object.js")
	if err == nil || !strings.Contains(err.Error(), "go object not found") {
		t.Fatalf("missing object error = %v", err)
	}
}
