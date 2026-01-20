package rpclient

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/xmx/aegis-common/muxlink/muxproto"
)

type Client struct {
	dia muxproto.Dialer
	cli *http.Client
	log *slog.Logger
}

func NewClient(dia muxproto.Dialer, log *slog.Logger) *Client {
	tran := newHTTPTransport(dia, log)
	cli := &http.Client{Transport: tran}

	return &Client{
		dia: dia,
		cli: cli,
		log: log,
	}
}

func (c *Client) HTTPClient() *http.Client {
	return c.cli
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.cli.Do(req)
}

func (c *Client) Transport() http.RoundTripper {
	return c.cli.Transport
}

func (c *Client) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return c.dia.DialContext(ctx, network, address)
}
