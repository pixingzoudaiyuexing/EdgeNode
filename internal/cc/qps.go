package cc

// ReachedMinQPS 判断最近一分钟的请求数是否达到配置的最低平均 QPS。
// 官方文档定义：按一分钟平均值判断，0 表示不设置最低门槛。
func ReachedMinQPS(minQPSPerIP int, requestsLastMinute int64) bool {
	if minQPSPerIP <= 0 {
		return true
	}
	if requestsLastMinute < 0 {
		return false
	}
	return requestsLastMinute >= int64(minQPSPerIP)*60
}
