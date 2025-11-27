package crontab

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/metrics"
)

type MetricsConfigFunc func(ctx context.Context) (pushURL string, opts *metrics.PushOptions, err error)

func NewMetrics(this *model.Broker, cfg MetricsConfigFunc) cronv3.Tasker {
	id := this.ID.Hex()
	name := this.Name
	hostname, _ := os.Hostname()
	label := fmt.Sprintf(`instance="%s",instance_type="broker",instance_name="%s",hostname="%s"`, id, name, hostname)

	return &metricsTask{
		cfg:   cfg,
		label: label,
	}
}

type metricsTask struct {
	cfg   MetricsConfigFunc
	label string
}

func (mt *metricsTask) Info() cronv3.TaskInfo {
	return cronv3.TaskInfo{
		Name:      "上报系统指标",
		Timeout:   5 * time.Second,
		CronSched: cronv3.NewInterval(5 * time.Second),
	}
}

func (mt *metricsTask) Call(ctx context.Context) error {
	pushURL, opts, err := mt.cfg(ctx)
	if err != nil {
		return err
	}
	opts.ExtraLabels = mt.label

	return metrics.PushMetricsExt(ctx, pushURL, mt.defaultWrite, opts)
}

func (*metricsTask) defaultWrite(w io.Writer) {
	metrics.WritePrometheus(w, true)
	metrics.WriteFDMetrics(w)
}
