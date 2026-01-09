package restapi

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/errcode"
	"github.com/xmx/aegis-common/library/httpkit"
	"github.com/xmx/aegis-common/muxlink/muxproto"
	"github.com/xmx/aegis-common/problem"
)

func NewReverse(dial muxproto.Dialer) *Reverse {
	trip := &http.Transport{DialContext: dial.DialContext}
	resv := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetXForwarded()
		},
		Transport: trip,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			host := r.Host
			if host == "" {
				host = r.URL.Host
			}

			prob := &problem.Details{
				Host:     host,
				Status:   http.StatusBadGateway,
				Instance: r.URL.Path,
				Method:   r.Method,
				Datetime: time.Now().UTC(),
			}

			if ae, ok := err.(*net.OpError); ok {
				addr := ae.Addr.String()
				nodeID, _, found := strings.Cut(addr, muxproto.AgentHostSuffix)
				if found {
					err = errcode.FmtAgentDisconnect.Fmt(nodeID)
				}
			}
			prob.Detail = err.Error()

			_ = prob.JSON(w)
		},
	}
	wsu := &websocket.Upgrader{
		HandshakeTimeout:  10 * time.Second,
		CheckOrigin:       func(*http.Request) bool { return true },
		EnableCompression: true,
	}
	wsd := &websocket.Dialer{
		NetDialContext:    dial.DialContext,
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true,
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

	destURL := muxproto.BrokerToAgentURL(id, pth)
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
	srv, _, err := rvs.wsd.DialContext(ctx, destURL.String(), nil)
	if err != nil {
		c.Errorf("websocket 后端连接失败", "error", err)
		_ = rvs.writeClose(cli, err)
		return
	}
	defer srv.Close()

	ret := httpkit.ExchangeWebsocket(cli, srv)
	c.Infof("websocket 连接结束", slog.Any("result", ret))
}

func (rvs *Reverse) writeClose(cli *websocket.Conn, err error) error {
	return cli.WriteMessage(websocket.CloseMessage, []byte(err.Error()))
}
