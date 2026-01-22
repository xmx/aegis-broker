package rpclient

import (
	"context"
	"net/http"

	"github.com/xmx/aegis-common/muxlink/muxproto"
	"github.com/xmx/aegis-common/muxlink/muxtool"
)

type Client struct {
	base muxtool.Client
}

func NewClient(base muxtool.Client) Client {
	return Client{
		base: base,
	}
}

func (c Client) BaseClient() muxtool.Client {
	return c.base
}

func (c Client) Ping(ctx context.Context) error {
	reqURL := muxproto.ToServerURL("/api/health/ping")
	strURL := reqURL.String()

	return c.base.JSON(ctx, http.MethodGet, strURL, nil)
}
