package restapi

import (
	"net/http"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/server/response"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-control/datalayer/model"
)

func NewSystem(hide *config.Config, boot model.BrokerConfig) *System {
	return &System{
		hide: hide,
		boot: boot,
	}
}

type System struct {
	hide *config.Config
	boot model.BrokerConfig
}

func (syt *System) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/system/config").GET(syt.config)
	r.Route("/system/ping").GET(syt.ping)
	return nil
}

func (syt *System) config(c *ship.Context) error {
	ret := &response.SystemConfig{
		Hide: syt.hide,
		Boot: syt.boot,
	}

	return c.JSON(http.StatusOK, ret)
}

func (*System) ping(c *ship.Context) error {
	return c.NoContent(http.StatusNoContent)
}
