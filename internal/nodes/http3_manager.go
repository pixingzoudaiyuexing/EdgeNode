package nodes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/TeaOSLab/EdgeCommon/pkg/nodeconfigs"
	teaconst "github.com/TeaOSLab/EdgeNode/internal/const"
	"github.com/TeaOSLab/EdgeNode/internal/events"
	"github.com/TeaOSLab/EdgeNode/internal/remotelogs"
	"github.com/mssola/useragent"
	"github.com/quic-go/quic-go"
	quichttp3 "github.com/quic-go/quic-go/http3"
)

const http3ManagerLogType = "HTTP3_MANAGER"

type http3ServerInstance struct {
	port       int
	server     *quichttp3.Server
	packetConn net.PacketConn
	closed     atomic.Bool
}

func (this *http3ServerInstance) Close() {
	if this == nil || this.closed.Swap(true) {
		return
	}
	if this.server != nil {
		_ = this.server.Close()
	}
	if this.packetConn != nil {
		_ = this.packetConn.Close()
	}
}

// HTTP3Manager 管理 HTTP/3 UDP 监听器，并把请求交给现有 HTTPListener 处理。
// 自维护版本直接使用项目已经依赖的 quic-go，不重建原 Plus 私有的 QUIC 流适配层。
type HTTP3Manager struct {
	locker       sync.RWMutex
	servers      map[int]*http3ServerInstance
	httpListener *HTTPListener
}

var sharedHTTP3Manager *HTTP3Manager

func init() {
	if !teaconst.IsMain {
		return
	}

	sharedHTTP3Manager = NewHTTP3Manager()

	// 完整配置首次加载、整体刷新或部分网站刷新后，同步 HTTP/3 站点分组和监听端口。
	events.OnEvents([]events.Event{events.EventLoaded, events.EventReload, events.EventReloadSomeServers}, func() {
		if sharedNodeConfig == nil {
			return
		}
		if err := sharedHTTP3Manager.Update(sharedNodeConfig); err != nil {
			remotelogs.Error(http3ManagerLogType, "update HTTP/3 listeners failed: "+err.Error())
		}
		if sharedListenerManager != nil {
			sharedListenerManager.http3Listener = sharedHTTP3Manager.HTTPListener()
		}
	})

	events.OnClose(func() {
		sharedHTTP3Manager.Shutdown()
	})
}

func NewHTTP3Manager() *HTTP3Manager {
	listener := &HTTPListener{}
	listener.isHTTP = false
	listener.isHTTPS = true
	listener.isHTTP3 = true
	listener.addr = "HTTP3"

	return &HTTP3Manager{
		servers:      map[int]*http3ServerInstance{},
		httpListener: listener,
	}
}

func (this *HTTP3Manager) HTTPListener() *HTTPListener {
	return this.httpListener
}

// UpdateHTTPListener 更新 HTTP/3 请求使用的网站分组。
// HTTPListener 保持为同一个实例，以便连接统计和动态 TLS/SNI 匹配能够连续工作。
func (this *HTTP3Manager) UpdateHTTPListener(nodeConfig *nodeconfigs.NodeConfig) {
	this.locker.Lock()
	defer this.locker.Unlock()
	this.updateHTTPListenerLocked(nodeConfig)
}

func (this *HTTP3Manager) updateHTTPListenerLocked(nodeConfig *nodeconfigs.NodeConfig) {
	if this.httpListener == nil {
		this.httpListener = &HTTPListener{}
	}
	this.httpListener.isHTTP = false
	this.httpListener.isHTTPS = true
	this.httpListener.isHTTP3 = true
	this.httpListener.addr = "HTTP3"

	if nodeConfig == nil {
		this.httpListener.Group = nil
		return
	}
	this.httpListener.Group = nodeConfig.HTTP3Group()
}

// Update 按当前节点配置增量更新 HTTP/3 UDP 端口。
// 同一个端口只启动一个 quic-go http3.Server；策略关闭或端口变化时会关闭旧监听。
func (this *HTTP3Manager) Update(nodeConfig *nodeconfigs.NodeConfig) error {
	this.locker.Lock()
	defer this.locker.Unlock()

	this.updateHTTPListenerLocked(nodeConfig)

	desiredPorts := map[int]bool{}
	if nodeConfig != nil && nodeConfig.IsOn && this.httpListener != nil && this.httpListener.Group != nil && len(this.httpListener.Group.Servers()) > 0 {
		for _, policy := range nodeConfig.HTTP3Policies {
			if policy == nil || !policy.IsOn {
				continue
			}
			port := policy.Port
			if port <= 0 {
				port = nodeconfigs.DefaultHTTP3Port
			}
			desiredPorts[port] = true
		}
	}

	for port, instance := range this.servers {
		if desiredPorts[port] {
			continue
		}
		instance.Close()
		delete(this.servers, port)
		remotelogs.Println(http3ManagerLogType, "close UDP port :"+strconv.Itoa(port))
	}

	var firstErr error
	for port := range desiredPorts {
		if _, ok := this.servers[port]; ok {
			continue
		}
		instance, err := this.createServer(port)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			remotelogs.Error(http3ManagerLogType, "listen UDP port :"+strconv.Itoa(port)+" failed: "+err.Error())
			continue
		}
		this.servers[port] = instance
		remotelogs.Println(http3ManagerLogType, "listen UDP port :"+strconv.Itoa(port))
	}

	return firstErr
}

func (this *HTTP3Manager) createServer(port int) (*http3ServerInstance, error) {
	if port <= 0 {
		port = nodeconfigs.DefaultHTTP3Port
	}
	if this.httpListener == nil || this.httpListener.Group == nil {
		return nil, errors.New("HTTP/3 listener group is not ready")
	}

	addr := ":" + strconv.Itoa(port)
	packetConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}

	instance := &http3ServerInstance{
		port:       port,
		packetConn: packetConn,
	}

	server := &quichttp3.Server{
		Addr:      addr,
		Port:      port,
		TLSConfig: this.httpListener.buildTLSConfig(),
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			this.httpListener.ServeHTTPWithAddr(writer, request, "udp://:"+strconv.Itoa(port))
		}),
		ConnContext: func(ctx context.Context, conn quic.Connection) context.Context {
			atomic.AddInt64(&this.httpListener.countActiveConnections, 1)
			go func() {
				<-conn.Context().Done()
				atomic.AddInt64(&this.httpListener.countActiveConnections, -1)
			}()
			return ctx
		},
	}
	instance.server = server

	go func() {
		err := server.Serve(packetConn)
		if err == nil || instance.closed.Load() || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return
		}
		remotelogs.Error(http3ManagerLogType, fmt.Sprintf("serve UDP port :%d failed: %s", port, err.Error()))
	}()

	return instance, nil
}

func (this *HTTP3Manager) Shutdown() {
	if this == nil {
		return
	}
	this.locker.Lock()
	defer this.locker.Unlock()
	for port, instance := range this.servers {
		instance.Close()
		delete(this.servers, port)
	}
}

func (this *HTTP3Manager) IsOn() bool {
	if this == nil {
		return false
	}
	this.locker.RLock()
	defer this.locker.RUnlock()
	return len(this.servers) > 0
}

// ProcessHTTP3Headers 为普通 HTTPS 响应添加 quic-go 生成的 Alt-Svc。
// supportMobileBrowsers 关闭时仅抑制对移动端主动宣告 HTTP/3，不阻断客户端直接发起的 HTTP/3 连接。
func (this *HTTP3Manager) ProcessHTTP3Headers(request *HTTPRequest, responseHeader http.Header) {
	if this == nil || request == nil || request.ReqServer == nil || request.nodeConfig == nil || responseHeader == nil {
		return
	}

	policy := request.nodeConfig.FindHTTP3PolicyWithClusterId(request.ReqServer.ClusterId)
	if policy == nil || !policy.IsOn {
		return
	}

	if !policy.SupportMobileBrowsers && request.RawReq != nil && useragent.New(request.RawReq.UserAgent()).Mobile() {
		return
	}

	port := policy.Port
	if port <= 0 {
		port = nodeconfigs.DefaultHTTP3Port
	}

	this.locker.RLock()
	instance := this.servers[port]
	this.locker.RUnlock()
	if instance == nil || instance.server == nil || instance.closed.Load() {
		return
	}

	// 设置 Port 后 quic-go 会只宣告当前集群策略要求的 UDP 端口，并自动使用本版本支持的 H3 协议标识。
	_ = instance.server.SetQuicHeaders(responseHeader)
}
