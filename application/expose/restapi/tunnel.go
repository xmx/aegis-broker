package restapi

import (
	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/library/httpkit"
	"github.com/xmx/aegis-common/muxlink/muxconn"
	"github.com/xmx/aegis-common/muxlink/muxproto"
)

type Tunnel struct {
	acpt muxproto.MUXAccepter
	wsup *websocket.Upgrader
}

func NewTunnel(acpt muxproto.MUXAccepter) *Tunnel {
	return &Tunnel{
		acpt: acpt,
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
	proto := c.Query("protocol")

	var mux muxconn.Muxer
	if proto == "smux" {
		mux, err = muxconn.NewSMUX(conn, nil, true)
	} else {
		mux, err = muxconn.NewYaMUX(conn, nil, true)
	}
	if err != nil {
		_ = conn.Close()
		return err
	}
	tnl.acpt.AcceptMUX(mux)

	return nil
}
