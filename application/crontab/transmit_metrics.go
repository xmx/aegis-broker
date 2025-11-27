package crontab

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/tunnel/tunopen"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/linkhub"
	"github.com/xmx/metrics"
)

func NewTransmitMetrics(this *model.Broker, mux tunopen.Muxer, hub linkhub.Huber, cfg MetricsConfigFunc) cronv3.Tasker {
	id := this.ID.Hex()
	name := this.Name
	hostname, _ := os.Hostname()
	label := fmt.Sprintf(`instance="%s",instance_type="broker",instance_name="%s",hostname="%s"`, id, name, hostname)
	return &transmitMetrics{
		mux:       mux,
		hub:       hub,
		cfg:       cfg,
		brokLabel: label,
	}
}

type transmitMetrics struct {
	mux       tunopen.Muxer
	hub       linkhub.Huber
	cfg       MetricsConfigFunc
	brokLabel string
}

func (t *transmitMetrics) Info() cronv3.TaskInfo {
	return cronv3.TaskInfo{
		Name:      "上报通道传输数据量指标",
		Timeout:   10 * time.Second,
		CronSched: cronv3.NewInterval(10 * time.Second),
	}
}

func (t *transmitMetrics) Call(ctx context.Context) error {
	pushURL, opts, err := t.cfg(ctx)
	if err != nil {
		return err
	}

	return metrics.PushMetricsExt(ctx, pushURL, t.write, opts)
}

func (t *transmitMetrics) write(w io.Writer) {
	t.writeBroker(w)
	t.writeAgent(w)
}

func (t *transmitMetrics) writeBroker(w io.Writer) {
	rx, tx := t.mux.Transferred()
	rxName := fmt.Sprintf("tunnel_receive_bytes{%s}", t.brokLabel)
	txName := fmt.Sprintf("tunnel_transmit_bytes{%s}", t.brokLabel)
	metrics.WriteCounterUint64(w, rxName, rx)
	metrics.WriteCounterUint64(w, txName, tx)
}

func (t *transmitMetrics) writeAgent(w io.Writer) {
	for _, p := range t.hub.Peers() {
		label := t.getAgentLabel(p)
		rxName := fmt.Sprintf("tunnel_receive_bytes{%s}", label)
		txName := fmt.Sprintf("tunnel_transmit_bytes{%s}", label)
		rx, tx := t.mux.Transferred()

		// 在 broker 端统计 agent 的传输数据，rx tx 要互换
		metrics.WriteCounterUint64(w, rxName, tx)
		metrics.WriteCounterUint64(w, txName, rx)
	}
}

func (t *transmitMetrics) getAgentLabel(p linkhub.Peer) string {
	id := p.ID().Hex()
	inf := p.Info()
	pattern := `instance="%s",instance_type="agent",goos="%s",goarch="%s",hostname="%s",inet="%s"`
	return fmt.Sprintf(pattern, id, inf.Goos, inf.Goarch, inf.Hostname, inf.Inet)
}
