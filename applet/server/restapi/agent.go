package restapi

import (
	"net/http"
	"net/http/httputil"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/transport"
	"github.com/xmx/aegis-control/library/httpnet"
)

type Agent struct {
	prx *httputil.ReverseProxy
}

func NewAgent(trip http.RoundTripper) *Agent {
	prx := httpnet.NewReverse(trip)
	return &Agent{
		prx: prx,
	}
}

func (agt *Agent) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/reverse/agent/:id/").Any(agt.reverse)
	r.Route("/reverse/agent/:id/*path").Any(agt.reverse)
	return nil
}

func (agt *Agent) reverse(c *ship.Context) error {
	id, pth := c.Param("id"), "/"+c.Param("path")
	w, r := c.Response(), c.Request()
	reqURL := transport.NewAgentURL(id, pth)
	r.URL = reqURL
	r.Host = reqURL.Host

	agt.prx.ServeHTTP(w, r)

	return nil
}
