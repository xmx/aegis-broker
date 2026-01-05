package crontab

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/metrics"
)

type MetricsConfigFunc func(ctx context.Context) (pushURL string, opts *metrics.PushOptions, err error)

func NewMetrics(this *model.Broker, cfg MetricsConfigFunc) cronv3.Tasker {
	id := this.ID.Hex()
	name := this.Name
	hostname, _ := os.Hostname()
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	label := fmt.Sprintf(`instance="%s",instance_type="broker",instance_name="%s",hostname="%s",goos="%s",goarch="%s"`, id, name, hostname, goos, goarch)

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
		CronSched: cron.Every(5 * time.Second),
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
