package restapi

import (
	"log/slog"

	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/library/wsocket"
	"github.com/xmx/aegis-common/transport"
)

type Tunnel struct {
	next transport.Handler
	wsup *websocket.Upgrader
}

func NewTunnel(next transport.Handler) *Tunnel {
	return &Tunnel{
		next: next,
		wsup: wsocket.NewUpgrade(),
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
		return err
	}
	conn := ws.NetConn()
	mux, err := transport.NewSMUX(conn, true)
	if err != nil {
		_ = conn.Close()
		return err
	}

	if err = tnl.next.Handle(mux); err != nil {
		c.Infof("处理发生错误", slog.Any("error", err))
	}

	return nil
}
