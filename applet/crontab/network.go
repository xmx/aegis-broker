package crontab

import (
	"context"
	"time"

	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/system/network"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func NewNetwork(id bson.ObjectID, repo repository.All) cronv3.Tasker {
	return &networkCard{
		id:   id,
		repo: repo,
	}
}

type networkCard struct {
	id   bson.ObjectID
	repo repository.All
	last network.Cards
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
		return nil
	}

	update := bson.M{"$set": bson.M{"networks": cards}}
	repo := n.repo.Broker()
	if _, err := repo.UpdateByID(ctx, n.id, update); err != nil {
		return err
	}

	n.last = cards

	return nil
}
