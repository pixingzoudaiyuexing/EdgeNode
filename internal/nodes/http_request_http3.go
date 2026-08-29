// Copyright 2023 GoEdge CDN goedge.cdn@gmail.com. All rights reserved. Official site: https://goedge.cn .

package nodes

import "net/http"

// processHTTP3Headers 为支持 HTTP/3 的 HTTPS 站点添加 Alt-Svc 响应头。
// 具体端口、移动端宣告策略和当前监听状态统一由 HTTP3Manager 判断。
func (this *HTTPRequest) processHTTP3Headers(respHeader http.Header) {
	if sharedHTTP3Manager == nil {
		return
	}
	sharedHTTP3Manager.ProcessHTTP3Headers(this, respHeader)
}
