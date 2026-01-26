package restapi

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/channel/rpclient"
	"github.com/xmx/aegis-common/muxlink/muxproto"
	"github.com/xmx/aegis-common/wsocket"
)

func NewReverse(cli rpclient.Client) *Reverse {
	base := cli.BaseClient()

	resv := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetXForwarded()
		},
		Transport: base.Transport(),
	}
	wsu := &websocket.Upgrader{
		HandshakeTimeout:  10 * time.Second,
		CheckOrigin:       func(*http.Request) bool { return true },
		EnableCompression: true,
	}
	wsd := &websocket.Dialer{
		NetDialContext: base.DialContext,
	}

	return &Reverse{
		prx: resv,
		wsu: wsu,
		wsd: wsd,
	}
}

type Reverse struct {
	prx *httputil.ReverseProxy
	wsu *websocket.Upgrader
	wsd *websocket.Dialer
}

func (rvs *Reverse) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/reverse/:id/").Any(rvs.serve)
	r.Route("/reverse/:id/*path").Any(rvs.serve)
	return nil
}

func (rvs *Reverse) serve(c *ship.Context) error {
	id, pth := c.Param("id"), "/"+c.Param("path")
	w, r := c.Response(), c.Request()

	reqURL := r.URL
	beforePath := reqURL.Path
	if pth != "/" && strings.HasSuffix(beforePath, "/") {
		pth += "/"
	}

	destURL := muxproto.ToAgentURL(id, pth)
	destURL.RawQuery = reqURL.RawQuery

	if c.IsWebSocket() {
		rvs.serveWebsocket(c, destURL)
		return nil
	}

	r.URL = destURL
	r.Host = reqURL.Host
	rvs.prx.ServeHTTP(w, r)

	return nil
}

//goland:noinspection GoUnhandledErrorResult
func (rvs *Reverse) serveWebsocket(c *ship.Context, destURL *url.URL) {
	w, r := c.Response(), c.Request()
	ctx := r.Context()

	cli, err := rvs.wsu.Upgrade(w, r, nil)
	if err != nil {
		c.Errorf("websocket upgrade 失败", "error", err)
		return
	}
	defer cli.Close()

	destURL.Scheme = "ws"
	strURL := destURL.String()
	srv, _, err := rvs.wsd.DialContext(ctx, strURL, nil)
	if err != nil {
		c.Errorf("连接 agent 后端失败", "url", strURL, "error", err)
		_ = rvs.writeClose(cli, err)
		return
	}
	defer srv.Close()

	ret := wsocket.Exchange(cli, srv)
	c.Infof("websocket 连接结束", slog.Any("result", ret))
}

func (rvs *Reverse) writeClose(cli *websocket.Conn, err error) error {
	return cli.WriteMessage(websocket.CloseMessage, []byte(err.Error()))
}
