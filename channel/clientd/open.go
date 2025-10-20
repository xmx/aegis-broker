package clientd

import (
	"context"
	"encoding/json/v2"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"time"

	"github.com/xmx/aegis-common/library/timex"
	"github.com/xmx/aegis-common/options"
	"github.com/xmx/aegis-common/tunnel/tundial"
	"github.com/xmx/aegis-common/tunnel/tunutil"
	"github.com/xmx/aegis-control/contract/authmesg"
)

func Open(cfg tundial.Config, secret string, opts ...options.Lister[option]) (tundial.Muxer, *authmesg.ServerToBrokerResponse, error) {
	if cfg.Parent == nil {
		cfg.Parent = context.Background()
	}
	opt := options.Eval(opts...)

	ac := &brokerClient{cfg: cfg, opt: opt, secret: secret}
	mux, res, err := ac.opens()
	if err != nil {
		return nil, nil, err
	}

	amux := tundial.MakeAtomic(mux)
	ac.mux = amux
	go ac.serve(mux)

	return amux, res, nil
}

type brokerClient struct {
	cfg    tundial.Config
	opt    option
	mux    tundial.AtomicMuxer
	secret string
}

func (bc *brokerClient) opens() (tundial.Muxer, *authmesg.ServerToBrokerResponse, error) {
	req := &authmesg.BrokerToServerRequest{
		Secret: bc.secret,
		Goos:   runtime.GOOS,
		Goarch: runtime.GOARCH,
		PID:    os.Getpid(),
		Args:   os.Args,
	}
	req.Workdir, _ = os.Getwd()
	req.Executable, _ = os.Executable()
	req.Hostname, _ = os.Hostname()
	if cu, _ := user.Current(); cu != nil {
		req.Username = cu.Username
		req.UID = cu.Uid
	}

	var reties int
	startedAt := time.Now()
	for {
		reties++ // 连接次数累加

		mux, res, err := bc.open(req)
		if err == nil && res.Succeed() {
			return mux, res, nil
		}

		sleep := bc.backoff(time.Since(startedAt), reties)
		bc.log().Warn("等待重连", "reties", reties, "sleep", sleep, "error", err)
		if err = timex.Sleep(bc.cfg.Parent, sleep); err != nil {
			bc.log().Error("不满足继续重试连接的条件", "error", err)
			return nil, nil, err
		}
	}
}

func (bc *brokerClient) open(req *authmesg.BrokerToServerRequest) (tundial.Muxer, *authmesg.ServerToBrokerResponse, error) {
	mux, err := tundial.Open(bc.cfg)
	if err != nil {
		bc.log().Info("基础网络连接失败", "error", err)
		return nil, nil, err
	}

	protocol, subprotocol := mux.Protocol()
	attrs := []any{
		slog.Any("local_addr", mux.Addr()),
		slog.Any("remote_addr", mux.RemoteAddr()),
		slog.Any("protocol", protocol),
		slog.Any("subprotocol", subprotocol),
	}
	bc.log().Info("基础网络连接成功", attrs...)

	res, err1 := bc.authentication(mux, req, time.Minute)
	if err1 != nil {
		attrs = append(attrs, slog.Any("error", err1))
	}
	if res != nil {
		attrs = append(attrs, slog.Any("auth_response", res))
	}
	if err1 == nil && res != nil && res.Succeed() {
		bc.log().Info("通道连接认证成功", attrs...)
		return mux, res, nil
	}

	_ = mux.Close() // 关闭连接
	bc.log().Warn("基础网络连接成功但认证失败", attrs...)

	return nil, res, err1
}

func (bc *brokerClient) authentication(mux tundial.Muxer, req *authmesg.BrokerToServerRequest, timeout time.Duration) (*authmesg.ServerToBrokerResponse, error) {
	ctx, cancel := context.WithTimeout(bc.cfg.Parent, timeout)
	defer cancel()

	conn, err := mux.Open(ctx)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer conn.Close()

	now := time.Now()
	_ = conn.SetDeadline(now.Add(timeout))
	if err = tunutil.WriteHead(conn, req); err != nil {
		return nil, err
	}
	resp := new(authmesg.ServerToBrokerResponse)
	err = tunutil.ReadHead(conn, resp)

	return resp, err
}

func (bc *brokerClient) serve(mux tundial.Muxer) {
	srv := bc.opt.server
	if srv == nil {
		srv = &http.Server{Handler: http.NotFoundHandler()}
	}

	const sleep = 2 * time.Second
	for {
		err := srv.Serve(mux)
		_ = mux.Close()

		bc.log().Warn("broker 通道掉线了", "error", err, "sleep", sleep)
		ctx := bc.cfg.Parent
		_ = timex.Sleep(ctx, sleep)
		if mux, _, err = bc.opens(); err != nil {
			break
		}

		bc.mux.Swap(mux)
	}
}

func (*brokerClient) writeAuthRequest(c net.Conn, v any) error {
	return json.MarshalWrite(c, v)
}

func (*brokerClient) readAuthResponse(c net.Conn) (*authResponse, error) {
	res := new(authResponse)
	if err := json.UnmarshalRead(c, res); err != nil {
		return nil, err
	}

	return res, nil
}

// backoff 通过持续连接耗费的时间和次数，计算出一个合理的重试时间。
func (*brokerClient) backoff(elapsed time.Duration, reties int) time.Duration {
	if reties < 60 {
		return 3 * time.Second
	} else if reties < 200 {
		return 10 * time.Second
	} else if reties < 500 {
		return 20 * time.Second
	} else if reties < 1000 {
		return time.Minute
	}

	const mouth = 30 * 24 * time.Hour
	if elapsed < mouth {
		return time.Minute
	}

	return 10 * time.Minute
}

func (bc *brokerClient) log() *slog.Logger {
	if l := bc.opt.logger; l != nil {
		return l
	}

	return slog.Default()
}
