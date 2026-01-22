package serverd

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/xmx/aegis-common/muxlink/muxconn"
	"github.com/xmx/aegis-common/muxlink/muxproto"
	"github.com/xmx/aegis-common/muxlink/muxtool"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func New(repo repository.All, opts Options) muxproto.MUXAccepter {
	return &agentServer{
		repo: repo,
		opts: opts,
	}
}

type agentServer struct {
	repo repository.All
	opts Options
}

// AcceptMUX 处理连接。
//
//goland:noinspection GoUnhandledErrorResult
func (as *agentServer) AcceptMUX(mux muxconn.Muxer) {
	defer mux.Close()

	connectAt := time.Now()
	if !as.limitAllowed(mux) {
		as.log().Warn("限流器抑制上线", "remote_addr", mux.RemoteAddr())
	}

	peer, err := as.authentication(mux)
	if err != nil {
		raddr := mux.RemoteAddr()
		as.log().Warn("节点上线失败", "remote_addr", raddr, "error", err)
		return
	}

	info := peer.Info()
	as.log().Info("节点上线成功", "info", info)
	if sh := as.opts.ServerHooker; sh != nil {
		sh.OnConnected(info, connectAt)
	}

	err = as.serveHTTP(peer)
	as.log().Warn("节点下线了", "info", info, "error", err)

	as.disconnection(peer, connectAt)
}

//goland:noinspection GoUnhandledErrorResult
func (as *agentServer) authentication(mux muxconn.Muxer) (linkhub.Peer, error) {
	timeout := as.timeout()

	fc := muxtool.NewFlagCloser(mux)
	timer := time.AfterFunc(timeout, fc.Close)
	conn, err := mux.Accept()
	timer.Stop()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if fc.Closed() {
		return nil, net.ErrClosed
	}

	req := new(AuthRequest)
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	if err = muxtool.ReadAuth(conn, req); err != nil {
		as.log().Warn("读取认证报文错误", "error", err)
		return nil, err
	}

	attrs := []any{"request", req}
	if err = as.validAuthRequest(req); err != nil {
		attrs = append(attrs, "error", err)
		as.log().Warn("认证报文参数校验错误", attrs...)
		as.responseError(conn, err, 0)
		return nil, err
	}

	agt, err := as.findOrCreateAgent(req)
	if err != nil {
		as.log().Warn("查询/新增节点错误", "error", err)
		as.responseError(conn, err, 0)
		return nil, err
	}

	// 在线状态检查
	if agt.Status {
		err = errors.New("此节点已经在线了（数据库）")
		as.log().Warn("节点重复上线（数据库）", attrs...)
		as.responseError(conn, err, http.StatusConflict)

		return nil, err
	}

	agentID := agt.ID
	info := linkhub.Info{
		Name: req.MachineID, Inet: req.Inet, Goos: req.Goos, Goarch: req.Goarch,
		Hostname: req.Hostname, Semver: req.Semver,
	}
	peer := as.putHuber(agentID, mux, info)
	if peer == nil {
		err = errors.New("此节点已经在线了（连接池）")
		as.log().Warn("节点重复上线（连接池）", attrs...)
		as.responseError(conn, err, http.StatusConflict)

		return nil, err
	}

	if err = as.responseAccepted(conn); err != nil {
		as.deleteHuber(agentID) // 报文响应失败，从连接池中删除并返回错误。

		attrs = append(attrs, "error", err)
		as.log().Warn("通过报文写入失败", attrs...)
		as.responseError(conn, err, http.StatusConflict)

		return nil, err
	}

	// 修改数据库在线状态
	if ret, err2 := as.updateAgentOnline(mux, req, agt); err2 != nil || ret.ModifiedCount == 0 {
		as.deleteHuber(agentID) // 修改数据库状态失败，从连接池中删除并返回错误。

		if err2 == nil {
			err2 = errors.New("没有找到该节点（修改在线状态）")
		}
		attrs = append(attrs, "error", err)
		as.log().Error("节点重复上线（连接池）", attrs...)
		as.responseError(conn, err2, http.StatusConflict)

		return nil, err
	}

	return peer, nil
}

func (as *agentServer) limitAllowed(mux muxconn.Muxer) bool {
	if l := as.opts.Limiter; l != nil {
		return l(mux)
	}

	return true
}

func (as *agentServer) log() *slog.Logger {
	if l := as.opts.Logger; l != nil {
		return l
	}

	return slog.Default()
}

func (as *agentServer) disconnection(peer linkhub.Peer, connectAt time.Time) {
	disconnectAt := time.Now()
	id := peer.ID()
	info := peer.Info()
	mux := peer.Muxer()
	tx, rx := mux.Traffic() // 互换

	attrs := []any{"info", info}
	filter := bson.D{{"_id", id}, {"status", true}}
	update := bson.M{"$set": bson.M{
		"status": false, "tunnel_stat.disconnected_at": disconnectAt,
		"tunnel_stat.receive_bytes": rx, "tunnel_stat.transmit_bytes": tx,
		// 注意：此时是站在 broker 视角统计的流量，所以 rx tx 要互换一下。
	}}

	ctx, cancel := as.perContext()
	defer cancel()

	repo := as.repo.Agent()
	if ret, err := repo.UpdateOne(ctx, filter, update); err != nil {
		attrs = append(attrs, "error", err)
		as.log().Error("修改数据库节点下线状态错误", attrs...)
	} else if ret.ModifiedCount == 0 {
		as.log().Error("修改数据库节点下线状态无修改", attrs...)
	}

	as.deleteHuber(id)

	libName, libModule := mux.Library()
	raddr, laddr := mux.Addr(), mux.RemoteAddr() // 互换
	second := int64(disconnectAt.Sub(connectAt).Seconds())
	history := &model.AgentConnectHistory{
		AgentID:   id,
		MachineID: info.Name,
		Semver:    info.Semver,
		Inet:      info.Inet,
		Goos:      info.Goos,
		Goarch:    info.Goarch,
		TunnelStat: model.TunnelStatHistory{
			ConnectedAt:    connectAt,
			DisconnectedAt: disconnectAt,
			Second:         second,
			Library:        model.TunnelLibrary{Name: libName, Module: libModule},
			LocalAddr:      laddr.String(),
			RemoteAddr:     raddr.String(),
			ReceiveBytes:   rx,
			TransmitBytes:  tx,
		},
	}
	hisRepo := as.repo.AgentConnectHistory()
	if _, err := hisRepo.InsertOne(ctx, history); err != nil {
		attrs = append(attrs, "save_history_error", err)
		as.log().Error("保存连接历史记录错误", attrs...)
	}

	as.log().Info("节点下线处理完毕", attrs...)

	if sh := as.opts.ServerHooker; sh != nil {
		sh.OnDisconnected(info, connectAt, disconnectAt)
	}
}

func (as *agentServer) timeout() time.Duration {
	if du := as.opts.Timeout; du > 0 {
		return du
	}

	return time.Minute
}

func (as *agentServer) perContext() (context.Context, context.CancelFunc) {
	du := as.timeout()
	ctx := as.opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithTimeout(ctx, du)
}

func (as *agentServer) validAuthRequest(req *AuthRequest) error {
	if v := as.opts.Validator; v != nil {
		return v(req)
	}
	// TODO 兜底的参数校验逻辑

	return nil
}

func (as *agentServer) responseError(conn net.Conn, err error, code int) error {
	if code < http.StatusBadRequest {
		code = http.StatusBadRequest
	}
	dat := &authResponse{Code: code, Message: err.Error()}

	d := as.timeout()
	_ = conn.SetWriteDeadline(time.Now().Add(d))

	return muxtool.WriteAuth(conn, dat)
}

func (as *agentServer) responseAccepted(conn net.Conn) error {
	dat := &authResponse{Code: http.StatusAccepted}

	d := as.timeout()
	_ = conn.SetWriteDeadline(time.Now().Add(d))

	return muxtool.WriteAuth(conn, dat)
}

// checkout 获得 agent 节点的信息，如果不存在自动创建。
func (as *agentServer) findOrCreateAgent(req *AuthRequest) (*model.Agent, error) {
	machineID := req.MachineID
	repo := as.repo.Agent()

	ctx, cancel := as.perContext()
	defer cancel()

	filter := bson.D{{"machine_id", machineID}}
	agt, err := repo.FindOne(ctx, filter)
	if err == nil {
		return agt, nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	now := time.Now()
	data := &model.Agent{
		MachineID: machineID,
		Status:    false, // 新增时默认离线状态
		CreatedAt: now,
		UpdatedAt: now,
	}
	ret, err := repo.InsertOne(ctx, data)
	if err != nil {
		return nil, err
	}
	id, _ := ret.InsertedID.(bson.ObjectID)
	data.ID = id

	return data, nil
}

func (as *agentServer) updateAgentOnline(mux muxconn.Muxer, req *AuthRequest, agt *model.Agent) (*mongo.UpdateResult, error) {
	// 修改数据库在线状态
	now := time.Now()
	id := agt.ID
	libName, libModule := mux.Library()
	raddr, laddr := mux.Addr(), mux.RemoteAddr() // 互换
	tx, rx := mux.Traffic()                      // 互换
	tunStat := &model.TunnelStat{
		ConnectedAt:   now,
		KeepaliveAt:   now,
		Library:       model.TunnelLibrary{Name: libName, Module: libModule},
		ReceiveBytes:  rx,
		TransmitBytes: tx,
		LocalAddr:     laddr.String(),
		RemoteAddr:    raddr.String(),
	}
	exeStat := &model.ExecuteStat{
		Inet:       req.Inet,
		Goos:       req.Goos,
		Goarch:     req.Goarch,
		Semver:     req.Semver,
		PID:        req.PID,
		Args:       req.Args,
		Hostname:   req.Hostname,
		Workdir:    req.Workdir,
		Executable: req.Executable,
	}
	point := &model.AgentConnectedBroker{
		ID:   as.opts.CurrentBroker.ID,
		Name: as.opts.CurrentBroker.Name,
	}

	update := bson.M{"$set": bson.M{
		"status": true, "tunnel_stat": tunStat, "execute_stat": exeStat, "broker": point,
	}}
	filter := bson.D{{"_id", id}, {"status", false}}

	ctx, cancel := as.perContext()
	defer cancel()

	repo := as.repo.Agent()

	return repo.UpdateOne(ctx, filter, update)
}

func (as *agentServer) putHuber(id bson.ObjectID, mux muxconn.Muxer, inf linkhub.Info) linkhub.Peer {
	return as.opts.Huber.Put(id, mux, inf)
}

func (as *agentServer) deleteHuber(id bson.ObjectID) {
	as.opts.Huber.DelID(id)
}

func (as *agentServer) serveHTTP(peer linkhub.Peer) error {
	h := as.opts.Handler
	if h == nil {
		h = http.NotFoundHandler()
	}

	srv := &http.Server{
		Handler: h,
		BaseContext: func(net.Listener) context.Context {
			return linkhub.WithValue(context.Background(), peer)
		},
	}
	mux := peer.Muxer()

	return srv.Serve(mux)
}
