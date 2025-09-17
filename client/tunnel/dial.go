package tunnel

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/xmx/aegis-common/library/timex"
	"github.com/xmx/aegis-common/transport"
	"github.com/xmx/aegis-control/contract/sbmesg"
)

type Subscriber interface {
	OnDisconnected(err error, disconnAt time.Time)
	OnReconnected(mux transport.Muxer, disconnAt, connAt time.Time)
	OnExit(err error)
}

type Client interface {
	Muxer() transport.Muxer
	Close() error
	InitialConfig(context.Context) (*sbmesg.BrokerInitialConfig, error)
}

type DialConfig struct {
	ID         string
	Secret     string
	Addresses  []string
	Handler    http.Handler
	DialConfig transport.DialConfig
	Subscriber Subscriber
	Timeout    time.Duration
	Logger     *slog.Logger
}

func (d DialConfig) Open() (Client, error) {
	if d.Timeout <= 0 {
		d.Timeout = 30 * time.Second
	}
	if d.DialConfig.Parent == nil {
		d.DialConfig.Parent = context.Background()
	}
	if len(d.Addresses) == 0 {
		d.Addresses = []string{"localhost:443", transport.ServerHost}
	}

	size := len(d.Addresses)
	uniq := make(map[string]struct{}, size)
	addrs := make([]string, 0, size)
	for _, addr := range d.Addresses {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "443")
		}
		if _, exists := uniq[addr]; !exists {
			uniq[addr] = struct{}{}
			addrs = append(addrs, addr)
		}
	}
	d.Addresses = addrs

	ml := transport.NewMuxLoader(nil)
	hc := &http.Client{
		Transport: transport.NewHTTPTransport(ml, transport.ServerHost),
	}

	cli := &brokerClient{
		cfg: d,
		mux: ml,
		cli: hc,
	}
	mux, err := cli.connect()
	if err != nil {
		return nil, err
	}
	go cli.serve(mux)

	return cli, nil
}

type brokerClient struct {
	cfg DialConfig
	mux transport.MuxLoader
	cli *http.Client
}

func (bc *brokerClient) Muxer() transport.Muxer {
	mux, _ := bc.mux.LoadMux()
	return mux
}

func (bc *brokerClient) Close() error {
	if mux := bc.Muxer(); mux != nil {
		return mux.Close()
	}

	return nil
}

func (bc *brokerClient) InitialConfig(ctx context.Context) (*sbmesg.BrokerInitialConfig, error) {
	reqURL := transport.NewServerURL("/api/config")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := bc.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	code := resp.StatusCode
	if code/100 != 2 { // 2xx
		// TODO
	}

	ret := new(sbmesg.BrokerInitialConfig)
	dec := jsontext.NewDecoder(resp.Body)
	if err = json.UnmarshalDecode(dec, ret); err != nil {
		return nil, err
	}

	return ret, nil
}

func (bc *brokerClient) connect() (transport.Muxer, error) {
	req := sbmesg.BrokerRequest{
		ID:     bc.cfg.ID,
		Goos:   runtime.GOOS,
		Goarch: runtime.GOARCH,
		Secret: bc.cfg.Secret,
	}

	var retry int
	timeout := bc.cfg.Timeout
	ctx := bc.cfg.DialConfig.Parent
	startAt := time.Now()
	for {
		for _, addr := range bc.cfg.Addresses {
			mux, err := bc.open(addr, req, timeout)
			if err == nil {
				proto := mux.Protocol()
				bc.log().Info("连接中心端成功", "addr", addr, "protocol", proto)
				bc.mux.StoreMux(mux)
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
			bc.log().Warn("连接中心端失败", attrs...)

			if exx := timex.Sleep(ctx, wait); exx != nil {
				return nil, exx
			}
		}
	}
}

func (bc *brokerClient) open(addr string, req sbmesg.BrokerRequest, timeout time.Duration) (transport.Muxer, error) {
	dial := bc.cfg.DialConfig
	mux, err := dial.DialTimeout(addr, timeout)
	if err != nil {
		return nil, err
	}

	if err = bc.handshake(mux, req, timeout); err != nil {
		_ = mux.Close()
		return nil, err
	}

	return mux, nil
}

func (bc *brokerClient) handshake(mux transport.Muxer, req sbmesg.BrokerRequest, timeout time.Duration) error {
	parent := bc.cfg.DialConfig.Parent
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	sig, err := mux.Open(ctx)
	if err != nil {
		return err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer sig.Close()

	deadline := time.Now().Add(timeout)
	_ = sig.SetDeadline(deadline)
	if err = json.MarshalEncode(jsontext.NewEncoder(sig), req); err != nil {
		return err
	}
	resp := new(sbmesg.BrokerResponse)
	if err = json.UnmarshalDecode(jsontext.NewDecoder(sig), resp); err != nil {
		return err
	}
	if resp.Succeed {
		return nil
	}

	return resp
}

func (bc *brokerClient) waitN(startAt time.Time, retry int) time.Duration {
	du := time.Since(startAt)
	if du < time.Minute {
		return time.Second
	} else if du < 5*time.Minute {
		return 3 * time.Second
	} else if du < 30*time.Minute {
		return 10 * time.Second
	} else {
		return time.Minute
	}
}

func (bc *brokerClient) serve(mux transport.Muxer) {
	for {
		hand := bc.cfg.Handler
		srv := &http.Server{Handler: hand}
		err := srv.Serve(mux) // 连接正常会阻塞在这里
		_ = mux.Close()

		disconnAt := time.Now()
		sub := bc.cfg.Subscriber
		if sub != nil {
			sub.OnDisconnected(err, disconnAt)
		}

		bc.log().Warn("节点掉线了", slog.Any("error", err))
		if mux, err = bc.connect(); err != nil {
			bc.log().Error("连接失败，不再尝试重连", slog.Any("error", err))
			if sub != nil {
				sub.OnExit(err)
			}
			break
		} else {
			bc.log().Info("重连成功")
			connAt := time.Now()
			if sub != nil {
				sub.OnReconnected(mux, disconnAt, connAt)
			}
		}
	}
}

func (bc *brokerClient) log() *slog.Logger {
	if l := bc.cfg.Logger; l != nil {
		return l
	}

	return slog.Default()
}
