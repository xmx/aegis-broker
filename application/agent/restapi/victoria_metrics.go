package restapi

import (
	"context"
	"fmt"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/business"
	"github.com/xmx/aegis-control/linkhub"
	"github.com/xmx/aegis-control/victoria"
	"github.com/xmx/metrics"
)

func NewVictoriaMetrics(svc *business.VictoriaMetrics) *VictoriaMetrics {
	return &VictoriaMetrics{svc: svc}
}

type VictoriaMetrics struct {
	svc *business.VictoriaMetrics
}

func (vm *VictoriaMetrics) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/victoria-metrics/write").GET(vm.write)
	return nil
}

func (vm *VictoriaMetrics) write(c *ship.Context) error {
	w, r := c.Response(), c.Request()
	ctx := r.Context()
	peer, _ := linkhub.FromContext(ctx)

	prx := victoria.NewProxy(vm.pushConfigFunc(peer))
	prx.ServeHTTP(w, r)

	return nil
}

func (vm *VictoriaMetrics) pushConfigFunc(peer linkhub.Peer) func(ctx context.Context) (string, *metrics.PushOptions, error) {
	return func(ctx context.Context) (string, *metrics.PushOptions, error) {
		pushURL, opts, err := vm.svc.PushConfig(ctx)
		if err != nil {
			return "", nil, err
		}

		id := peer.ID().Hex()
		inf := peer.Info()
		pattern := `instance="%s",instance_type="agent",goos="%s",goarch="%s",hostname="%s",inet="%s"`
		label := fmt.Sprintf(pattern, id, inf.Goos, inf.Goarch, inf.Hostname, inf.Inet)
		opts.ExtraLabels = label

		return pushURL, opts, nil
	}
}
