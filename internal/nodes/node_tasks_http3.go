package nodes

import (
	"encoding/json"
	"fmt"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	"github.com/TeaOSLab/EdgeCommon/pkg/rpc/pb"
	"github.com/TeaOSLab/EdgeNode/internal/remotelogs"
	"github.com/TeaOSLab/EdgeNode/internal/rpc"
)

// execHTTP3PolicyChangedTask 增量刷新当前节点所属集群的 HTTP/3 策略。
// 更新完成后立即同步 UDP 监听端口，并刷新自动防火墙端口集合。
func (this *Node) execHTTP3PolicyChangedTask(rpcClient *rpc.RPCClient) error {
	remotelogs.Println("NODE", "updating HTTP/3 policies ...")

	resp, err := rpcClient.NodeRPC.FindNodeHTTP3Policies(rpcClient.Context(), &pb.FindNodeHTTP3PoliciesRequest{})
	if err != nil {
		return err
	}

	policies := map[int64]*nodeconfigs.HTTP3Policy{}
	for _, item := range resp.Http3Policies {
		if item == nil || len(item.Http3PolicyJSON) == 0 {
			continue
		}

		policy := nodeconfigs.NewHTTP3Policy()
		err = json.Unmarshal(item.Http3PolicyJSON, policy)
		if err != nil {
			return fmt.Errorf("decode HTTP/3 policy failed: %w", err)
		}
		err = policy.Init()
		if err != nil {
			return fmt.Errorf("initialize HTTP/3 policy failed: %w", err)
		}
		policies[item.NodeClusterId] = policy
	}

	if sharedNodeConfig == nil {
		return nil
	}
	sharedNodeConfig.UpdateHTTP3Policies(policies)

	if sharedHTTP3Manager != nil {
		err = sharedHTTP3Manager.Update(sharedNodeConfig)
		if sharedListenerManager != nil {
			sharedListenerManager.http3Listener = sharedHTTP3Manager.HTTPListener()
		}
	}

	// 集群 HTTP/3 端口可能发生变化，需要同步 firewalld 中的 UDP 端口。
	if sharedListenerManager != nil {
		sharedListenerManager.reloadFirewalld()
	}

	return err
}
