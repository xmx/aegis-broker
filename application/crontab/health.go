package crontab

import (
	"context"
	"net/http"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xmx/aegis-broker/channel/rpclient"
	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/muxlink/muxproto"
)

func NewHealth(cli *rpclient.Client) cronv3.Tasker {
	return &healthPing{
		cli: cli,
	}
}

type healthPing struct {
	cli *rpclient.Client
}

func (hp *healthPing) Info() cronv3.TaskInfo {
	return cronv3.TaskInfo{
		Name:      "发送心跳包",
		Timeout:   5 * time.Second,
		CronSched: cron.Every(time.Minute),
	}
}

func (hp *healthPing) Call(ctx context.Context) error {
	reqURL := muxproto.ToServerURL("/api/health/ping")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return err
	}

	res, err := hp.cli.Do(req)
	if err != nil {
		return err
	}
	_ = res.Body.Close()

	return nil
}
