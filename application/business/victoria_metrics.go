package business

import (
	"context"
	"log/slog"

	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/library/memoize"
	"github.com/xmx/metrics"
)

func NewVictoriaMetrics(repo repository.All, this *model.Broker, log *slog.Logger) *VictoriaMetrics {
	return &VictoriaMetrics{
		repo: repo,
		this: this,
		log:  log,
		cfg:  memoize.NewCache2(repo.VictoriaMetrics().Enabled),
	}
}

type VictoriaMetrics struct {
	repo repository.All
	this *model.Broker
	log  *slog.Logger
	cfg  memoize.Cache2[*model.VictoriaMetrics, error]
}

func (vm *VictoriaMetrics) Reset() {
	_, _ = vm.cfg.Forget()
}

func (vm *VictoriaMetrics) PushConfig(ctx context.Context) (string, *metrics.PushOptions, error) {
	cfg, err := vm.cfg.Load(ctx)
	if err != nil {
		return "", nil, err
	}

	headers := make([]string, 0, len(cfg.Header))
	for k, v := range cfg.Header {
		headers = append(headers, k+": "+v)
	}

	opts := &metrics.PushOptions{
		Headers: headers,
		Method:  cfg.Method,
		Client:  nil,
	}

	return cfg.Address, opts, nil
}
