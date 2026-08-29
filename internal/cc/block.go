package cc

import (
	"github.com/TeaOSLab/EdgeCommon/pkg/serverconfigs/firewallconfigs"
	"github.com/TeaOSLab/EdgeNode/internal/utils/fasttime"
	"github.com/TeaOSLab/EdgeNode/internal/waf"
)

// RecordBlockedIP 将已经判定需要封禁的客户端 IP 写入 1.3.9 原生临时黑名单。
//
// 这里故意不负责判断请求是否达到 CC 阈值，也不自行决定 Firewall Scope 或
// 是否调用本机防火墙。上述行为属于 Plus CC 策略语义，在证据确认前由调用方
// 显式传入，避免把尚未确认的行为固化到公共基础设施中。
func RecordBlockedIP(serverId int64, remoteAddr string, scope firewallconfigs.FirewallScope, blockSeconds int, useLocalFirewall bool, reason string) int64 {
	return recordBlockedIP(waf.SharedIPBlackList, serverId, remoteAddr, scope, blockSeconds, useLocalFirewall, reason)
}

func recordBlockedIP(list *waf.IPList, serverId int64, remoteAddr string, scope firewallconfigs.FirewallScope, blockSeconds int, useLocalFirewall bool, reason string) int64 {
	if list == nil || serverId <= 0 || len(remoteAddr) == 0 || blockSeconds <= 0 {
		return 0
	}

	expiresAt := fasttime.Now().Unix() + int64(blockSeconds)
	list.RecordIP(
		waf.IPTypeAll,
		scope,
		serverId,
		remoteAddr,
		expiresAt,
		0,
		useLocalFirewall,
		0,
		0,
		reason,
	)
	return expiresAt
}
