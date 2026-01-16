package restapi

import (
	"net/http/httputil"
	"net/url"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/labelset"
	"github.com/xmx/aegis-control/linkhub"
)

type Pyroscope struct {
	prx *httputil.ReverseProxy
}

func NewPyroscope() *Pyroscope {
	pu, _ := url.Parse("https://pyroscope.example.com")
	prx := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(pu)
		},
	}
	return &Pyroscope{
		prx: prx,
	}
}

func (prs *Pyroscope) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/pyroscope/ingest").POST(prs.ingest)
	return nil
}

func (prs *Pyroscope) ingest(c *ship.Context) error {
	w, r := c.Response(), c.Request()
	ctx := r.Context()
	peer, _ := linkhub.FromContext(ctx)

	quires := r.URL.Query()
	labelSet, err := labelset.Parse(quires.Get("name"))
	if err != nil {
		return err
	}
	labels := labelSet.Labels()

	inf := peer.Info()
	labels["instance"] = peer.ID().Hex()
	labels["instance_type"] = "agent"
	labels["goos"] = inf.Goos
	labels["goarch"] = inf.Goarch
	labels["hostname"] = inf.Hostname
	labels["inet"] = inf.Inet
	str := labelset.New(labels).LabelSet()
	quires.Set("name", str)
	r.URL.Path = "/ingest"
	r.URL.RawQuery = quires.Encode()

	prs.prx.ServeHTTP(w, r)

	return nil
}
