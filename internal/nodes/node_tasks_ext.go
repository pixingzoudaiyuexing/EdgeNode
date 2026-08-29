// Copyright 2023 GoEdge CDN goedge.cdn@gmail.com. All rights reserved. Official site: https://goedge.cn .
//go:build !plus

package nodes

import "github.com/TeaOSLab/EdgeNode/internal/rpc"

func (this *Node) execScriptsChangedTask() error {
	// stub
	return nil
}

// UAM 与 HTTP CC 的任务刷新已迁移到无 build tag 的自维护实现，
// 普通构建和后续自维护 plus 构建共用同一条安全策略刷新链。

func (this *Node) execHTTPPagesPolicyChangedTask(rpcClient *rpc.RPCClient) error {
	// stub
	return nil
}

func (this *Node) execNetworkSecurityPolicyChangedTask(rpcClient *rpc.RPCClient) error {
	// stub
	return nil
}

func (this *Node) execPlanChangedTask(rpcClient *rpc.RPCClient) error {
	return nil
}
