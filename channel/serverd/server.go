package serverd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/xmx/aegis-common/muxlink/muxconn"
	"github.com/xmx/aegis-common/muxlink/muxproto"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func New(cur *model.Broker, repo repository.All, opts Options) muxproto.MUXAccepter {
	return &agentServer{
		repo: repo,
		cur:  cur,
		opt:  opts,
	}
}

type agentServer struct {
	repo repository.All
	cur  *model.Broker // 当前 broker 的信息
	opt  Options
}

// AcceptMUX 处理连接。
//
//goland:noinspection GoUnhandledErrorResult
func (as *agentServer) AcceptMUX(mux muxconn.Muxer) {
	defer mux.Close()

	raddr := mux.RemoteAddr()
	attrs := []any{"remote_addr", raddr}
	if !as.opt.allowed(mux) {
		as.opt.log().Warn("限流器抑制上线", attrs...)
		return
	}

	timeout := as.opt.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	_, peer, succeed := as.authentication(mux, timeout)
	if !succeed {
		return
	}
	defer as.disconnected(peer, timeout)

	srv := as.getServer(peer)
	_ = srv.Serve(mux)
}

// authentication 节点认证。
// 客户端主动建立一条虚拟子流连接用于交换认证信息，认证后改子流关闭，后续子流即为业务流。
//
//goland:noinspection GoUnhandledErrorResult
func (as *agentServer) authentication(mux muxconn.Muxer, timeout time.Duration) (*AuthRequest, linkhub.Peer, bool) {
	protocol, subprotocol := mux.Protocol()
	laddr, raddr := mux.Addr(), mux.RemoteAddr()
	attrs := []any{
		slog.String("protocol", protocol), slog.String("subprotocol", subprotocol),
		slog.Any("local_addr", laddr), slog.Any("remote_addr", raddr),
	}

	safem := &mutuallyClose{mux: mux}
	timer := time.AfterFunc(timeout, safem.close) // 设置超时主动断开，防止恶意客户端一直不建立认证连接。
	defer timer.Stop()

	sig, err := mux.Accept()
	timer.Stop()
	if err != nil {
		as.opt.log().Error("等待 agent 建立认证通道出错", "error", err)
		return nil, nil, false
	}
	defer sig.Close()
	if safem.closed() {
		return nil, nil, false
	}

	// 读取数据
	now := time.Now()
	_ = sig.SetDeadline(now.Add(timeout))
	req := new(AuthRequest)
	if err = muxproto.ReadJSON(sig, req); err != nil {
		attrs = append(attrs, slog.Any("error", err))
		as.opt.log().Error("读取 agent 请求信息错误", attrs...)
		return nil, nil, false
	}
	attrs = append(attrs, slog.Any("auth_request", req))
	if err = as.opt.valid(req); err != nil {
		attrs = append(attrs, slog.Any("error", err))
		as.opt.log().Error("校验请求报文错误", attrs...)
		_ = as.writeError(sig, http.StatusBadRequest, err)
		return nil, nil, false
	}
	agt, err := as.checkout(req, timeout)
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
		as.opt.log().Error("查询/新增 agent 错误", attrs...)
		_ = as.writeError(sig, http.StatusInternalServerError, err)
		return nil, nil, false
	}
	if agt.Status { // 节点已经在线了
		as.opt.log().Warn("agent 重复上线（数据库检查）", attrs...)
		_ = as.writeError(sig, http.StatusConflict, nil)
		return nil, nil, false
	}

	pinf := linkhub.Info{Inet: req.Inet, Goos: req.Goos, Goarch: req.Goarch, Hostname: req.Hostname}
	peer := linkhub.NewPeer(agt.ID, mux, pinf)
	if !as.opt.Huber.Put(peer) {
		as.opt.log().Warn("agent 重复上线（连接池检查）", attrs...)
		_ = as.writeError(sig, http.StatusConflict, nil)
		return nil, nil, false
	}

	// 成功报文
	agtID := agt.ID
	if err = as.writeSucceed(sig); err != nil {
		as.opt.Huber.DelByID(agtID)
		attrs = append(attrs, slog.Any("error", err))
		as.opt.log().Warn("写入成功报文错误", attrs...)
		return nil, nil, false
	}

	// 修改数据库在线状态
	tunStat := &model.TunnelStat{
		ConnectedAt: now,
		KeepaliveAt: now,
		Protocol:    protocol,
		Subprotocol: subprotocol,
		LocalAddr:   laddr.String(),
		RemoteAddr:  raddr.String(),
	}
	exeStat := &model.ExecuteStat{
		Inet:       req.Inet,
		Goos:       req.Goos,
		Goarch:     req.Goarch,
		PID:        req.PID,
		Args:       req.Args,
		Hostname:   req.Hostname,
		Workdir:    req.Workdir,
		Executable: req.Executable,
	}
	point := &model.AgentConnectedBroker{
		ID:   as.cur.ID,
		Name: as.cur.Name,
	}
	update := bson.M{"$set": bson.M{
		"status": true, "tunnel_stat": tunStat, "execute_stat": exeStat, "broker": point,
	}}
	filter := bson.D{{"_id", agt.ID}, {"status", false}}
	agentRepo := as.repo.Agent()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ret, err1 := agentRepo.UpdateOne(ctx, filter, update)
	if err1 == nil && ret.ModifiedCount != 0 {
		as.opt.log().Info("agent 上线成功", attrs...)
		return req, peer, true
	}

	as.opt.Huber.DelByID(agtID)
	_ = as.writeError(sig, http.StatusInternalServerError, err1)

	if err1 != nil {
		attrs = append(attrs, slog.Any("error", err1))
	}
	as.opt.log().Error("agent 上线失败", attrs...)

	return nil, nil, false
}

// checkout 获得 agent 节点的信息，如果不存在自动创建。
func (as *agentServer) checkout(req *AuthRequest, timeout time.Duration) (*model.Agent, error) {
	machineID := req.MachineID
	agentRepo := as.repo.Agent()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	agt, err := agentRepo.FindOne(ctx, bson.M{"machine_id": machineID})
	if err == nil {
		return agt, nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	now := time.Now()
	data := &model.Agent{
		MachineID: req.MachineID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	ret, err := agentRepo.InsertOne(ctx, data)
	if err != nil {
		return nil, err
	}
	id, _ := ret.InsertedID.(bson.ObjectID)
	data.ID = id

	return data, nil
}

func (as *agentServer) disconnected(peer linkhub.Peer, timeout time.Duration) {
	now := time.Now()
	id := peer.ID()
	rx, tx := peer.Muxer().Traffic()
	update := bson.M{"$set": bson.M{
		"status": false, "tunnel_stat.disconnected_at": now,
		"tunnel_stat.receive_bytes": tx, "tunnel_stat.transmit_bytes": rx,
		// 注意：此时要以 agent 视角统计流量，所以 rx tx 要互换一下。
	}}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	agentRepo := as.repo.Agent()
	_, _ = agentRepo.UpdateByID(ctx, id, update)
	as.opt.Huber.DelByID(id)
}

func (as *agentServer) getServer(p linkhub.Peer) *http.Server {
	h := as.opt.Handler
	if h == nil {
		h = http.NotFoundHandler()
	}

	return &http.Server{
		Handler: h,
		BaseContext: func(net.Listener) context.Context {
			return linkhub.WithValue(context.Background(), p)
		},
	}
}

func (as *agentServer) writeError(w io.Writer, code int, err error) error {
	resp := &authResponse{Code: code}
	if err != nil {
		resp.Message = err.Error()
	}

	return muxproto.WriteJSON(w, resp)
}

func (as *agentServer) writeSucceed(w io.Writer) error {
	resp := &authResponse{Code: http.StatusAccepted}
	return muxproto.WriteJSON(w, resp)
}

type mutuallyClose struct {
	mux  muxconn.Muxer
	mtx  sync.Mutex
	kill bool
}

func (m *mutuallyClose) close() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	_ = m.mux.Close()
	m.kill = true
}

func (m *mutuallyClose) closed() bool {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	return m.kill
}
