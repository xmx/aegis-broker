package restapi

import (
	"net/http"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/config"
)

func NewSystem(cfg *config.Config) *System {
	return &System{
		cfg: cfg,
	}
}

type System struct {
	cfg *config.Config
}

func (syt *System) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/system/config").GET(syt.config)
	r.Route("/system/ping").GET(syt.ping)
	return nil
}

func (syt *System) config(c *ship.Context) error {
	return c.JSON(http.StatusOK, syt.cfg)
}

func (*System) ping(c *ship.Context) error {
	return c.NoContent(http.StatusNoContent)
}
