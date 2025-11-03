package clientd

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/user"
	"runtime"
	"time"

	"github.com/xmx/aegis-common/library/timex"
	"github.com/xmx/aegis-common/options"
	"github.com/xmx/aegis-common/tunnel/tundial"
	"github.com/xmx/aegis-common/tunnel/tunutil"
)

func Open(cfg tundial.Config, secret string, opts ...options.Lister[option]) (tundial.Muxer, AuthConfig, error) {
	if cfg.Parent == nil {
		cfg.Parent = context.Background()
	}
	opts = append(opts, fallbackOptions())
	opt := options.Eval(opts...)

	mux := new(safeMuxer)
	ac := &brokerClient{cfg: cfg, opt: opt, mux: mux, secret: secret}
	authCfg, err := ac.opens()
	if err != nil {
		return nil, authCfg, err
	}

	go ac.serve(mux)

	return mux, authCfg, nil
}

type brokerClient struct {
	cfg    tundial.Config
	opt    option
	mux    *safeMuxer
	secret string
}

func (bc *brokerClient) opens() (AuthConfig, error) {
	req := &authRequest{
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
	}

	var fails int
	timeout := bc.cfg.PerTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	for {
		mux, res, err := bc.open(req, timeout)
		if err == nil {
			bc.mux.store(mux)
			return res.Config, nil
		}

		fails++
		wait := bc.waitTime(fails)
		bc.log().Warn("等待重连", "fails", fails, "error", err)
		if err = timex.Sleep(bc.cfg.Parent, wait); err != nil {
			bc.log().Error("不满足继续重试连接的条件", "error", err)
			return AuthConfig{}, err
		}
	}
}

func (bc *brokerClient) open(req *authRequest, timeout time.Duration) (tundial.Muxer, *authResponse, error) {
	mux, err := tundial.Open(bc.cfg)
	if err != nil {
		bc.log().Info("基础网络连接失败", "error", err)
		return nil, nil, err
	}

	laddr, raddr := mux.Addr(), mux.RemoteAddr()
	protocol, subprotocol := mux.Protocol()
	attrs := []any{
		slog.Any("local_addr", laddr),
		slog.Any("remote_addr", raddr),
		slog.Any("protocol", protocol),
		slog.Any("subprotocol", subprotocol),
	}
	bc.log().Info("基础网络连接成功", attrs...)

	req.Inet = raddr.String()
	res, err1 := bc.authentication(mux, req, timeout)
	if err1 != nil {
		_ = mux.Close()
		attrs = append(attrs, slog.Any("error", err1))
		return nil, nil, err1
	}
	if err = res.checkError(); err != nil {
		_ = mux.Close()
		attrs = append(attrs, slog.Any("error", err))
		bc.log().Warn("基础网络连接成功但认证失败", attrs...)
		return nil, res, err
	}

	return mux, res, nil
}

func (bc *brokerClient) authentication(mux tundial.Muxer, req *authRequest, timeout time.Duration) (*authResponse, error) {
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
	if err = tunutil.WriteAuth(conn, req); err != nil {
		return nil, err
	}
	resp := new(authResponse)
	if err = tunutil.ReadAuth(conn, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (bc *brokerClient) serve(mux tundial.Muxer) {
	for {
		srv := bc.opt.server
		err := srv.Serve(mux)
		_ = mux.Close()

		bc.log().Warn("broker 通道掉线了", "error", err)
		if _, err = bc.opens(); err != nil {
			break
		}
	}
}

// waitTime 通过持续次数，计算出一个合理的重试时间。
func (*brokerClient) waitTime(fails int) time.Duration {
	if fails <= 100 {
		return 3 * time.Second
	} else if fails <= 300 {
		return 10 * time.Second
	} else if fails <= 500 {
		return 30 * time.Second
	}

	return time.Minute
}
func (bc *brokerClient) log() *slog.Logger {
	if l := bc.opt.logger; l != nil {
		return l
	}

	return slog.Default()
}

func (*brokerClient) checkIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	}

	return net.IPv4zero
}
