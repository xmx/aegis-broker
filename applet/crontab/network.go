package crontab

import (
	"context"
	"log/slog"
	"time"

	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/system/network"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func NewNetwork(thisID bson.ObjectID, repo repository.All, log *slog.Logger) cronv3.Tasker {
	return &networkCard{
		thisID: thisID,
		repo:   repo,
		log:    log,
	}
}

type networkCard struct {
	thisID bson.ObjectID
	repo   repository.All
	log    *slog.Logger
	last   []*network.Card
}

func (n *networkCard) Info() cronv3.TaskInfo {
	return cronv3.TaskInfo{
		Name:      "上报网卡信息",
		Timeout:   10 * time.Second,
		CronSched: cronv3.NewInterval(time.Hour),
	}
}

func (n *networkCard) Call(ctx context.Context) error {
	cards := network.Interfaces()
	if cards.Equal(n.last) {
		n.log.Debug("网卡信息未发生变化")
		return nil
	}

	n.last = cards
	update := bson.M{"$set": bson.M{"networks": cards}}
	repo := n.repo.Broker()
	_, err := repo.UpdateByID(ctx, n.thisID, update)

	return err
}
