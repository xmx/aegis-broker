package gateway

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/xmx/aegis-common/contract/bamesg"
	"github.com/xmx/aegis-common/transport"
	"github.com/xmx/aegis-common/validation"
	"github.com/xmx/aegis-control/contract/linkhub"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func New(thisBrk *model.Broker, repo repository.All, hub linkhub.Huber, valid *validation.Validate, next http.Handler, log *slog.Logger) transport.Handler {
	return &agentGateway{
		this:  thisBrk,
		repo:  repo,
		hub:   hub,
		valid: valid,
		next:  next,
		log:   log,
	}
}

type agentGateway struct {
	this  *model.Broker
	repo  repository.All
	hub   linkhub.Huber
	valid *validation.Validate
	next  http.Handler
	log   *slog.Logger
}

func (gw *agentGateway) Handle(mux transport.Muxer) error {
	//goland:noinspection GoUnhandledErrorResult
	defer mux.Close()

	const timeout = 10 * time.Second
	peer, req, err := gw.handshake(mux, timeout)
	if err != nil {
		gw.log.Error("agent 上线认证失败", "error", err)
		return err
	}

	attrs := []any{
		slog.String("id", peer.ObjectID().Hex()),
		slog.String("machine_id", req.MachineID),
		slog.String("goos", req.Goos),
		slog.String("goarch", req.Goarch),
		slog.String("protocol", mux.Protocol()),
		slog.String("remote_addr", mux.RemoteAddr().String()),
	}
	gw.log.Info("agent 上线", attrs...)

	defer func() {
		gw.disconnected(peer, timeout)
		gw.log.Warn("agent 下线", attrs...)
	}()

	srv := &http.Server{
		Handler: gw.next,
		BaseContext: func(net.Listener) context.Context {
			return linkhub.WithValue(context.Background(), peer)
		},
	}
	err = srv.Serve(mux)
	attrs = append(attrs, slog.Any("error", err))

	return nil
}

func (gw *agentGateway) handshake(mux transport.Muxer, timeout time.Duration) (linkhub.Peer, *bamesg.AgentRequest, error) {
	timer := time.AfterFunc(timeout, func() { _ = mux.Close() })
	sig, err := mux.Accept()
	timer.Stop()
	if err != nil {
		return nil, nil, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer sig.Close()

	now := time.Now()
	_ = sig.SetDeadline(now.Add(timeout))

	agt, req, err := gw.read(sig, timeout)
	if err != nil {
		_ = gw.write(sig, err)
		return nil, nil, err
	}
	if agt.Status {
		err = linkhub.ErrDuplicateConnection
		_ = gw.write(sig, err)
		return nil, nil, err
	}

	peer := linkhub.NewPeer(agt.ID, mux)
	if !gw.hub.Put(peer) {
		err = linkhub.ErrDuplicateConnection
		_ = gw.write(sig, err)
		return nil, nil, err
	}

	brk := &model.AgentConnectedBroker{ID: gw.this.ID, Name: gw.this.Name}
	proto, remoteAddr := mux.Protocol(), mux.RemoteAddr().String()
	filter := bson.M{"_id": agt.ID, "status": false}
	update := bson.M{"$set": bson.M{
		"goos":         req.Goos,
		"goarch":       req.Goarch,
		"status":       true,
		"broker":       brk,
		"alive_at":     now,
		"protocol":     proto,
		"remote_addr":  remoteAddr,
		"connected_at": now,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	agentRepo := gw.repo.Agent()
	ret, err := agentRepo.UpdateOne(ctx, filter, update)
	if err != nil {
		_ = gw.write(sig, err)
		return nil, nil, err
	} else if ret.MatchedCount == 0 {
		err = mongo.ErrNoDocuments
		_ = gw.write(sig, err)
		return nil, nil, err
	}
	_ = gw.write(sig, nil)

	return peer, req, nil
}

func (gw *agentGateway) read(sig net.Conn, timeout time.Duration) (*model.Agent, *bamesg.AgentRequest, error) {
	req := new(bamesg.AgentRequest)
	dec := jsontext.NewDecoder(sig)
	if err := json.UnmarshalDecode(dec, req); err != nil {
		return nil, nil, err
	}
	if err := gw.valid.Validate(req); err != nil {
		return nil, nil, err
	}

	machineID, goos, goarch := req.MachineID, req.Goos, req.Goarch
	attrs := []any{
		slog.String("machine_id", machineID),
		slog.String("goos", goos),
		slog.String("goarch", goarch),
	}

	agt, create, err := gw.checkout(req, timeout)
	if err != nil {
		return nil, nil, err
	}
	attrs = append(attrs, slog.String("id", agt.ID.Hex()))
	if create {
		gw.log.Info("新注册 agent 节点", attrs...)
	}

	return agt, req, nil
}

func (gw *agentGateway) write(sig net.Conn, err error) error {
	res := new(bamesg.AgentResponse)
	if err != nil {
		res.Message = err.Error()
	} else {
		res.Succeed = true
	}
	enc := jsontext.NewEncoder(sig)

	return json.MarshalEncode(enc, res)
}

// checkout 通过机器码查询 agent 信息，如果不存在则需要新增，返回的 bool 表示是否是新增的 agent。
func (gw *agentGateway) checkout(req *bamesg.AgentRequest, timeout time.Duration) (*model.Agent, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	machineID, goos, goarch := req.MachineID, req.Goos, req.Goarch
	agentRepo := gw.repo.Agent()
	agt, err := agentRepo.FindOne(ctx, bson.M{"machine_id": machineID})
	if err == nil {
		return agt, false, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, err
	}

	now := time.Now()
	mod := &model.Agent{
		MachineID: machineID,
		Goos:      goos,
		Goarch:    goarch,
		CreatedAt: now,
		UpdatedAt: now,
	}
	ret, err := agentRepo.InsertOne(ctx, mod)
	if err != nil {
		return nil, false, err
	}
	mod.ID, _ = ret.InsertedID.(bson.ObjectID)

	return mod, true, nil
}

func (gw *agentGateway) disconnected(p linkhub.Peer, timeout time.Duration) {
	now := time.Now()
	id, host := p.ObjectID(), p.Host()
	filter := bson.M{"_id": id, "status": true}
	update := bson.M{"$set": bson.M{
		"status":          false,
		"disconnected_at": now,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	agentRepo := gw.repo.Agent()
	_, _ = agentRepo.UpdateOne(ctx, filter, update)

	gw.hub.Del(host)
}
