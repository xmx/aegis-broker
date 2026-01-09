package clientd

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/xmx/aegis-common/muxlink/muxconn"
	"github.com/xmx/aegis-common/muxlink/muxproto"
)

type Options struct {
	Secret  string
	Semver  string
	Handler http.Handler
}

func Open(cfg muxconn.DialConfig, opt Options) (muxconn.Muxer, *AuthConfig, error) {
	if cfg.Context == nil {
		cfg.Context = context.Background()
	}

	req := &authRequest{
		Secret: opt.Secret,
		Semver: opt.Semver,
		Goos:   runtime.GOOS,
		Goarch: runtime.GOARCH,
		PID:    os.Getpid(),
		Args:   os.Args,
	}
	req.Workdir, _ = os.Getwd()
	req.Executable, _ = os.Executable()
	req.Hostname, _ = os.Hostname()

	mux := new(muxInstance)
	cli := &brokerClient{
		cfg:  cfg,
		opts: opt,
		mux:  mux,
		req:  req,
	}

	mc, auth, err := cli.openLoop()
	if err != nil {
		return nil, nil, err
	}
	mux.store(mc)

	go cli.serveHTTP()

	return mux, auth, nil
}

type brokerClient struct {
	cfg  muxconn.DialConfig
	opts Options
	mux  *muxInstance
	req  *authRequest
}

// openLoop 连接服务端直至成功或遇到不可重试的错误。
func (bc *brokerClient) openLoop() (muxconn.Muxer, *AuthConfig, error) {
	var tires int
	for {
		tires++

		attrs := []any{"tires", tires}
		if mux, cfg, err := bc.open(); err != nil {
			attrs = append(attrs, "error", err)
		} else {
			bc.log().Info("通道连接成功", attrs...)
			return mux, cfg, nil
		}

		du := bc.retryInterval(tires)
		attrs = append(attrs, "sleep", du)
		bc.log().Warn("通道连接失败，稍后重试", attrs...)

		if err := bc.sleep(du); err != nil {
			attrs = append(attrs, "final_error", err)
			bc.log().Error("通道连接遇到不可重试的错误", attrs...)
			return nil, nil, err
		}
	}
}

//goland:noinspection GoUnhandledErrorResult
func (bc *brokerClient) open() (muxconn.Muxer, *AuthConfig, error) {
	mux, err := muxconn.Open(bc.cfg)
	if err != nil {
		return nil, nil, err
	}

	laddr, raddr := mux.Addr(), mux.RemoteAddr()
	outboundIP := muxproto.Outbound(laddr, raddr)
	bc.req.Inet = outboundIP.String()

	ctx, cancel := bc.perContext()
	defer cancel()

	conn, err := mux.Open(ctx)
	if err != nil {
		_ = mux.Close()
		return nil, nil, err
	}
	defer conn.Close()

	timeout := bc.timeout()
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if err = muxproto.WriteJSON(conn, bc.req); err != nil {
		_ = mux.Close()
		return nil, nil, err
	}

	resp := new(authResponse)
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	if err = muxproto.ReadJSON(conn, resp); err != nil {
		_ = mux.Close()
		return nil, nil, err
	}

	if err = resp.checkError(); err != nil {
		_ = mux.Close()
		return nil, nil, err
	}

	return mux, resp.Config, nil
}

func (bc *brokerClient) serveHTTP() {
	h := bc.opts.Handler
	if h == nil {
		h = http.NotFoundHandler()
	}

	for {
		srv := &http.Server{Handler: h}
		err := srv.Serve(bc.mux)
		bc.log().Warn("通道断开连接了", "error", err)

		mc, _, err1 := bc.openLoop()
		if err1 != nil {
			break
		}

		bc.mux.store(mc)
	}
}

func (bc *brokerClient) log() *slog.Logger {
	if l := bc.cfg.Logger; l != nil {
		return l
	}

	return slog.Default()
}

func (bc *brokerClient) timeout() time.Duration {
	if d := bc.cfg.PerTimeout; d > 0 {
		return d
	}

	return time.Minute
}

func (bc *brokerClient) perContext() (context.Context, context.CancelFunc) {
	d := bc.timeout()
	return context.WithTimeout(bc.cfg.Context, d)
}

func (bc *brokerClient) retryInterval(tires int) time.Duration {
	if tires <= 100 {
		return 3 * time.Second
	} else if tires <= 300 {
		return 10 * time.Second
	} else if tires <= 500 {
		return 30 * time.Second
	}

	return time.Minute
}

func (bc *brokerClient) sleep(d time.Duration) error {
	ctx := bc.cfg.Context
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
