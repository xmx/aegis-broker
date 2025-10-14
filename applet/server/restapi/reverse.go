package restapi

import (
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/tunnel/tunutil"
	"github.com/xmx/aegis-control/library/httpnet"
)

type Reverse struct {
	prx *httputil.ReverseProxy
}

func NewReverse(trip http.RoundTripper) *Reverse {
	prx := httpnet.NewReverse(trip)
	return &Reverse{
		prx: prx,
	}
}

func (agt *Reverse) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/reverse/agent/:id/").Any(agt.reverse)
	r.Route("/reverse/agent/:id/*path").Any(agt.reverse)
	return nil
}

func (agt *Reverse) reverse(c *ship.Context) error {
	id, pth := c.Param("id"), "/"+c.Param("path")
	w, r := c.Response(), c.Request()

	beforePath := r.URL.Path
	if pth != "/" && strings.HasSuffix(beforePath, "/") {
		pth += "/"
	}

	reqURL := tunutil.ServerToBroker(id, pth)
	r.URL = reqURL
	r.Host = reqURL.Host

	agt.prx.ServeHTTP(w, r)

	return nil
}
