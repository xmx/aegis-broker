package restapi

import (
	"net/http"
	"time"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/server/service"
)

func NewSystem(svc *service.System) *System {
	return &System{
		svc: svc,
	}
}

type System struct {
	svc *service.System
}

func (syt *System) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/system/config").GET(syt.config)
	r.Route("/system/ping").GET(syt.ping)
	r.Route("/system/exit").GET(syt.exit)
	r.Route("/system/upgrade").GET(syt.upgrade)
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
