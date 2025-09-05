package restapi

import (
	"net/http/pprof"

	"github.com/xgfone/ship/v5"
)

func NewPprof() *Pprof {
	return &Pprof{}
}

type Pprof struct{}

func (prf *Pprof) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/pprof/index").GET(prf.index)
	r.Route("/pprof/cmdline").GET(prf.cmdline)
	r.Route("/pprof/symbol").GET(prf.symbol).POST(prf.symbol)
	r.Route("/pprof/profile").GET(prf.profile)
	r.Route("/pprof/trace").GET(prf.trace)
	r.Route("/pprof/:name").GET(prf.name)
	return nil
}

func (prf *Pprof) index(c *ship.Context) error {
	pprof.Index(c, c.Request())
	return nil
}

func (prf *Pprof) cmdline(c *ship.Context) error {
	pprof.Cmdline(c, c.Request())
	return nil
}

func (prf *Pprof) profile(c *ship.Context) error {
	pprof.Profile(c, c.Request())
	return nil
}

func (prf *Pprof) symbol(c *ship.Context) error {
	pprof.Symbol(c, c.Request())
	return nil
}

func (prf *Pprof) trace(c *ship.Context) error {
	pprof.Trace(c, c.Request())
	return nil
}

func (prf *Pprof) name(c *ship.Context) error {
	name := c.Param("name")
	pprof.Handler(name).ServeHTTP(c, c.Request())
	return nil
}
