package bclient

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/xmx/aegis-common/library/httpx"
	"github.com/xmx/aegis-common/library/timex"
	"github.com/xmx/aegis-common/transport"
)

func Open(parent context.Context, cfg DialConfig, next http.Handler, log *slog.Logger) (Client, error) {
	addrs := make([]string, 0, 10)
	uniq := make(map[string]struct{}, 8)
	for _, s := range cfg.Addresses {
		if s == "" {
			continue
		}
		if _, ok := uniq[s]; ok {
			continue
		}

		if _, _, err := net.SplitHostPort(s); err != nil {
			s = net.JoinHostPort(s, ":443")
		}

		uniq[s] = struct{}{}
		addrs = append(addrs, s)
	}
	if len(addrs) == 0 {
		addrs = append(addrs, "127.0.0.1:443", "server.aegis.internal:443")
	}

	bc := &brokerClient{
		cfg:    cfg,
		log:    log,
		next:   next,
		parent: parent,
	}

	mux, err := bc.connect()
	if err != nil {
		return nil, err
	}
	amux := transport.NewAtomic(mux)
	tran := transport.NewHTTPTransport(amux, func(addr string) bool {
		host, _, exx := net.SplitHostPort(addr)
		return exx == nil && transport.ServerHost == host
	})
	cli := httpx.Client{
		Client: &http.Client{
			Transport: tran,
		},
	}
	bc.cli = cli
	bc.mux = amux

	go bc.serve()

	return bc, nil
}

type Client interface {
	Config(ctx context.Context) (*Database, error)
}

type DialConfig struct {
	ID        string
	Secret    string
	Addresses []string
}

type brokerClient struct {
	cfg    DialConfig
	log    *slog.Logger
	next   http.Handler
	mux    transport.AtomicMuxer
	cli    httpx.Client
	parent context.Context
}

func (bc *brokerClient) Config(ctx context.Context) (*Database, error) {
	reqURL := transport.NewServerURL("/api/config")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := bc.cli.Do(req)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close()

	dat := new(Database)
	if err = json.NewDecoder(resp.Body).Decode(dat); err != nil {
		return nil, err
	}

	return dat, nil
}

func (bc *brokerClient) reconnect() error {
	mux, err := bc.connect()
	if err != nil {
		return err
	}

	bc.mux.Store(mux)

	return nil
}

func (bc *brokerClient) connect() (transport.Muxer, error) {
	req := transport.AuthRequest{
		ID:     bc.cfg.ID,
		Goos:   runtime.GOOS,
		Goarch: runtime.GOARCH,
		Secret: bc.cfg.Secret,
	}

	var retry int
	startAt := time.Now()
	ctx := bc.parent
	for {
		for _, addr := range bc.cfg.Addresses {
			mux, err := bc.open(addr, req)
			if err == nil {
				return mux, nil
			}
			if exx := ctx.Err(); exx != nil {
				return nil, exx
			}

			retry++
			wait := bc.waitN(startAt, retry)

			attrs := []any{
				slog.Int("retry", retry),
				slog.Any("error", err),
				slog.Duration("wait", wait),
				slog.String("addr", addr),
			}
			bc.log.Warn("连接中心端失败", attrs...)

			if exx := timex.Sleep(ctx, wait); exx != nil {
				return nil, exx
			}
		}
	}
}

func (bc *brokerClient) waitN(startAt time.Time, retry int) time.Duration {
	du := time.Since(startAt)
	if du < time.Minute {
		return 2 * time.Second
	} else if du < 10*time.Minute {
		return 5 * time.Second
	} else if du < time.Hour {
		return 15 * time.Second
	} else {
		return time.Minute
	}
}

func (bc *brokerClient) open(addr string, req transport.AuthRequest) (transport.Muxer, error) {
	const timeout = 10 * time.Second

	dial := new(transport.DualDialer)

	ctx, cancel := context.WithTimeout(bc.parent, timeout)
	mux, err := dial.DialContext(ctx, addr)
	cancel()
	if err != nil {
		return nil, err
	}
	if err = bc.handshake(mux, req, timeout); err != nil {
		_ = mux.Close()
		return nil, err
	}

	return mux, nil
}

func (bc *brokerClient) handshake(mux transport.Muxer, req transport.AuthRequest, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(bc.parent, timeout)
	defer cancel()

	sig, err := mux.Open(ctx)
	if err != nil {
		return err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer sig.Close()

	deadline := time.Now().Add(timeout)
	_ = sig.SetDeadline(deadline)
	if err = json.NewEncoder(sig).Encode(req); err != nil {
		return err
	}
	resp := new(transport.AuthResponse)
	if err = json.NewDecoder(sig).Decode(resp); err != nil {
		return err
	}
	if resp.Succeed {
		return nil
	}

	return resp
}

func (bc *brokerClient) serve() {
	bc.log.Info("连接成功")
	srv := &http.Server{Handler: bc.next}

	for {
		err := srv.Serve(bc.mux)
		bc.log.Warn("掉线了", slog.Any("error", err))
		_ = timex.Sleep(bc.parent, 2*time.Second)
		if err = bc.reconnect(); err != nil {
			bc.log.Error("重连失败", slog.Any("error", err))
		} else {
			bc.log.Info("重连成功")
		}
	}
}
