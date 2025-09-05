package restapi

import (
	"net/http"

	"github.com/xgfone/ship/v5"
)

func NewHealth() *Health {
	return &Health{}
}

type Health struct{}

func (hlt *Health) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/health/ping").GET(hlt.ping)
	return nil
}

func (hlt *Health) ping(c *ship.Context) error {
	return c.NoContent(http.StatusNoContent)
}
