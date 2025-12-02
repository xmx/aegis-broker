package crontab

import (
	"context"
	"errors"
	"time"

	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/tunnel/tunopen"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func NewTransmit(id bson.ObjectID, mux tunopen.Muxer, hub linkhub.Huber, repo repository.All) cronv3.Tasker {
	return &transmit{
		id:   id,
		mux:  mux,
		hub:  hub,
		repo: repo,
	}
}

type transmit struct {
	id   bson.ObjectID
	mux  tunopen.Muxer
	hub  linkhub.Huber
	repo repository.All
}

func (t *transmit) Info() cronv3.TaskInfo {
	return cronv3.TaskInfo{
		Name:      "记录通道传输字节数",
		Timeout:   5 * time.Minute,
		CronSched: cronv3.NewInterval(time.Minute),
	}
}

func (t *transmit) Call(ctx context.Context) error {
	var errs []error
	if err := t.broker(ctx); err != nil {
		errs = append(errs, err)
	}
	if es := t.agents(ctx); len(es) != 0 {
		errs = append(errs, es...)
	}

	return errors.Join(errs...)
}

func (t *transmit) broker(ctx context.Context) error {
	rx, tx := t.mux.Transferred()
	update := bson.M{"$set": bson.M{
		"tunnel_stat.receive_bytes":  rx,
		"tunnel_stat.transmit_bytes": tx,
	}}

	repo := t.repo.Broker()
	_, err := repo.UpdateByID(ctx, t.id, update)

	return err
}

func (t *transmit) agents(ctx context.Context) []error {
	const batch = 100

	var errs []error
	mods := make([]mongo.WriteModel, 0, batch)
	for _, p := range t.hub.Peers() {
		id := p.ID()
		mux := p.Muxer()
		rx, tx := mux.Transferred()
		// 在 broker 端统计 agent 的传输数据，rx tx 要互换
		update := bson.M{"$set": bson.M{
			"tunnel_stat.receive_bytes":  tx,
			"tunnel_stat.transmit_bytes": rx,
		}}

		filter := bson.D{
			{Key: "_id", Value: id},
			{Key: "status", Value: true},
		}
		mod := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update)
		mods = append(mods, mod)

		if len(mods) < batch {
			continue
		}

		if err := t.bulkWrite(ctx, mods); err != nil {
			errs = append(errs, err)
		}
		mods = mods[:0]
	}
	if err := t.bulkWrite(ctx, mods); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func (t *transmit) bulkWrite(ctx context.Context, mods []mongo.WriteModel) error {
	if len(mods) == 0 {
		return nil
	}

	opt := options.BulkWrite().SetOrdered(false)
	repo := t.repo.Agent()
	_, err := repo.BulkWrite(ctx, mods, opt)

	return err
}
