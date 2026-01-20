package restapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/server/service"
	"github.com/xmx/aegis-common/muxlink/muxconn"
	"golang.org/x/time/rate"
)

func NewSystem(mux muxconn.Muxer, svc *service.System) *System {
	return &System{
		mux: mux,
		svc: svc,
	}
}

type System struct {
	mux muxconn.Muxer
	svc *service.System
}

func (syt *System) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/system/config").GET(syt.config)
	r.Route("/system/ping").GET(syt.ping)
	r.Route("/system/exit").GET(syt.exit)
	r.Route("/system/upgrade").GET(syt.upgrade)
	r.Route("/system/download").GET(syt.download)
	r.Route("/system/limit").GET(syt.limit)
	r.Route("/system/setlimit").GET(syt.setlimit)
	r.Route("/system/streams").GET(syt.streams)
	return nil
}

func (syt *System) config(c *ship.Context) error {
	ret := syt.svc.Config()
	return c.JSON(http.StatusOK, ret)
}

func (*System) ping(c *ship.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (syt *System) exit(*ship.Context) error {
	return syt.svc.Exit(time.Second)
}

// upgrade 通知 broker 升级。
func (syt *System) upgrade(c *ship.Context) error {
	ctx := c.Request().Context()
	_, err := syt.svc.Upgrade(ctx)

	return err
}

func (syt *System) download(c *ship.Context) error {
	return c.Attachment("D:\\Users\\Administrator\\Downloads\\30g.bin", "")
}

func (syt *System) limit(c *ship.Context) error {
	limit := syt.mux.Limit()

	return c.JSON(http.StatusOK, map[string]any{"limit": limit})
}

func (syt *System) setlimit(c *ship.Context) error {
	str := c.Query("limit")
	num, _ := strconv.ParseInt(str, 10, 64)
	syt.mux.SetLimit(rate.Limit(num * 1024))

	return c.NoContent(http.StatusNoContent)
}

func (syt *System) streams(c *ship.Context) error {
	history, active := syt.mux.NumStreams()

	return c.JSON(http.StatusOK, map[string]any{"history": history, "active": active})
}
