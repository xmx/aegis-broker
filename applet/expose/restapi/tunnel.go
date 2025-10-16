package restapi

import (
	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/library/httpkit"
	"github.com/xmx/aegis-common/tunnel/tundial"
	"github.com/xmx/aegis-common/tunnel/tunutil"
)

type Tunnel struct {
	next tunutil.Handler
	wsup *websocket.Upgrader
}

func NewTunnel(next tunutil.Handler) *Tunnel {
	return &Tunnel{
		next: next,
		wsup: httpkit.NewWebsocketUpgrader(),
	}
}

func (tnl *Tunnel) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/tunnel").GET(tnl.open)
	return nil
}

func (tnl *Tunnel) open(c *ship.Context) error {
	w, r := c.ResponseWriter(), c.Request()
	ws, err := tnl.wsup.Upgrade(w, r, nil)
	if err != nil {
		c.Warnf("通道协议升级失败（websocket）", "error", err)
		return err
	}

	conn := ws.NetConn()
	mux, err1 := tundial.NewSMUX(conn, nil, true)
	if err1 != nil {
		_ = conn.Close()
		return err1
	}
	tnl.next.Handle(mux)

	return nil
}
