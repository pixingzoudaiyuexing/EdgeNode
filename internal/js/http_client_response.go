//go:build script

package js

import "net/http"

// JSNetHTTPClientLibraryResponse 是 1.3.9 Plus HTTP Client Response 的共享 Go 对象。
// Scripts-2 的 Response.send() 需要读取其首字段 *http.Response；HTTP Client Library
// 自身的构造、请求与 body 生命周期将在后续独立恢复。
type JSNetHTTPClientLibraryResponse struct {
	resp       *http.Response
	err        string
	body       []byte
	isBodyRead bool
}
