package restapi

import (
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/applet/agent/service"
	"github.com/xmx/aegis-common/contract/message"
	"github.com/xmx/aegis-control/contract/linkhub"
	"github.com/xmx/aegis-control/datalayer/model"
)

func NewSystem(svc *service.System) *System {
	return &System{
		svc: svc,
	}
}

type System struct {
	svc *service.System
}

func (s *System) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/system/network").POST(s.network)
	return nil
}

func (s *System) network(c *ship.Context) error {
	req := new(message.Data[model.NodeNetworks])
	if err := c.Bind(req); err != nil {
		return err
	}

	ctx := c.Request().Context()
	peer := linkhub.FromContext(ctx)

	return s.svc.Network(ctx, req.Data, peer)
}
