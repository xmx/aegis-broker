package crontab

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xmx/aegis-broker/channel/rpclient"
	"github.com/xmx/aegis-common/library/cronv3"
)

func NewHealth(cli rpclient.Client) cronv3.Tasker {
	return &healthPing{
		cli: cli,
	}
}

type healthPing struct {
	cli rpclient.Client
}

func (hp *healthPing) Info() cronv3.TaskInfo {
	return cronv3.TaskInfo{
		Name:      "发送心跳包",
		Timeout:   5 * time.Second,
		CronSched: cron.Every(time.Minute),
	}
}

func (hp *healthPing) Call(ctx context.Context) error {
	return hp.cli.Ping(ctx)
}
