package restapi

import (
	"net/http"
	"net/http/httputil"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-server/channel/broker"
)

type Agent struct {
	prx *httputil.ReverseProxy
}

func NewAgent(trap http.RoundTripper) *Agent {
	return &Agent{}
}

func (agt *Agent) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/agent/reverse/:id/").Any(agt.reverse)
	r.Route("/agent/reverse/:id/*path").Any(agt.reverse)
	return nil
}

func (agt *Agent) reverse(c *ship.Context) error {
	id, path := c.Param("id"), "/"+c.Param("path")
	w, r := c.Response(), c.Request()
	reqURL := broker.MakesBrokerURL(id, path)
	r.URL = reqURL
	r.Host = reqURL.Host

	agt.prx.ServeHTTP(w, r)

	return nil
}
