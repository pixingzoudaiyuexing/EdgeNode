package nodes

import (
	"encoding/json"
	"fmt"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/rpc/pb"
	"github.com/TeaOSLab/EdgeNode/internal/remotelogs"
	"github.com/TeaOSLab/EdgeNode/internal/rpc"
)

// execUAMPolicyChangedTask 增量刷新当前节点所属集群的 UAM 策略。
// UAM 运行时直接读取 sharedNodeConfig，因此替换策略映射即可立即生效，无需重启监听器。
func (this *Node) execUAMPolicyChangedTask(rpcClient *rpc.RPCClient) error {
	remotelogs.Println("NODE", "updating UAM policies ...")

	resp, err := rpcClient.NodeRPC.FindNodeUAMPolicies(rpcClient.Context(), &pb.FindNodeUAMPoliciesRequest{})
	if err != nil {
		return err
	}

	policies := map[int64]*nodeconfigs.UAMPolicy{}
	for _, item := range resp.UamPolicies {
		if item == nil || len(item.UamPolicyJSON) == 0 {
			continue
		}

		// 1.3.9 原任务按零值对象解码 UAM JSON；保持这一行为，避免缺失字段被本地默认值悄悄补齐。
		policy := &nodeconfigs.UAMPolicy{}
		if err = json.Unmarshal(item.UamPolicyJSON, policy); err != nil {
			return fmt.Errorf("decode UAM policy failed: %w", err)
		}
		if err = policy.Init(); err != nil {
			return fmt.Errorf("initialize UAM policy failed: %w", err)
		}
		policies[item.NodeClusterId] = policy
	}

	if sharedNodeConfig != nil {
		sharedNodeConfig.UpdateUAMPolicies(policies)
	}
	return nil
}

// execHTTPCCPolicyChangedTask 增量刷新当前节点所属集群的 HTTP CC 策略。
// NewHTTPCCPolicy 保留 1.3.9 对旧配置的兼容默认值，再由服务端 JSON 覆盖实际字段。
func (this *Node) execHTTPCCPolicyChangedTask(rpcClient *rpc.RPCClient) error {
	remotelogs.Println("NODE", "updating HTTP CC policies ...")

	resp, err := rpcClient.NodeRPC.FindNodeHTTPCCPolicies(rpcClient.Context(), &pb.FindNodeHTTPCCPoliciesRequest{})
	if err != nil {
		return err
	}

	policies := map[int64]*nodeconfigs.HTTPCCPolicy{}
	for _, item := range resp.HttpCCPolicies {
		if item == nil || len(item.HttpCCPolicyJSON) == 0 {
			continue
		}

		policy := nodeconfigs.NewHTTPCCPolicy()
		if err = json.Unmarshal(item.HttpCCPolicyJSON, policy); err != nil {
			return fmt.Errorf("decode HTTP CC policy failed: %w", err)
		}
		if err = policy.Init(); err != nil {
			return fmt.Errorf("initialize HTTP CC policy failed: %w", err)
		}
		policies[item.NodeClusterId] = policy
	}

	if sharedNodeConfig != nil {
		sharedNodeConfig.UpdateHTTPCCPolicies(policies)
	}
	return nil
}
