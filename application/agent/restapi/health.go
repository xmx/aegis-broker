package restapi

import (
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/agent/service"
	"github.com/xmx/aegis-control/linkhub"
)

type Health struct {
	svc *service.Health
}

func NewHealth(svc *service.Health) *Health {
	return &Health{
		svc: svc,
	}
}

func (hlt *Health) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/health/ping").GET(hlt.ping)

	return nil
}

func (hlt *Health) ping(c *ship.Context) error {
	ctx := c.Request().Context()
	peer, _ := linkhub.FromContext(ctx)

	return hlt.svc.Ping(ctx, peer)
}
