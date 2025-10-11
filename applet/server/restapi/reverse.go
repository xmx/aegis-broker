package restapi

import (
	"net/http"
	"net/http/httputil"

	"github.com/xgfone/ship/v5"
)

type Reverse struct {
	prx *httputil.ReverseProxy
}

func NewReverse(trip http.RoundTripper) *Reverse {
	//prx := httpnet.NewReverse(trip)
	//return &Reverse{
	//	prx: prx,
	//}
	return &Reverse{}
}

func (agt *Reverse) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/reverse/agent/:id/").Any(agt.reverse)
	r.Route("/reverse/agent/:id/*path").Any(agt.reverse)
	return nil
}

func (agt *Reverse) reverse(c *ship.Context) error {
	//id, pth := c.Param("id"), "/"+c.Param("path")
	//w, r := c.Response(), c.Request()
	//ctx := r.Context()
	//if e := ctx.Err(); e != nil {
	//	c.Errorf("context error", "error", e)
	//}
	//reqURL := transport.NewBrokerAgentURL(id, pth)
	//r.URL = reqURL
	//r.Host = reqURL.Host
	//
	//agt.prx.ServeHTTP(w, r)

	return nil
}
