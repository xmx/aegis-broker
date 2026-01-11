package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Health struct {
	repo repository.All
	log  *slog.Logger
}

func NewHealth(repo repository.All, log *slog.Logger) *Health {
	return &Health{
		repo: repo,
		log:  log,
	}
}

func (hlt *Health) Ping(ctx context.Context, peer linkhub.Peer) error {
	now := time.Now()
	id := peer.ID()
	filter := bson.D{{"_id", id}, {"status", true}}
	update := bson.M{"$set": bson.M{"tunnel_stat.keepalive_at": now}}

	repo := hlt.repo.Agent()
	if _, err := repo.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	info := peer.Info()
	attrs := []any{"info", info}
	hlt.log.Debug("agent 发来了心跳", attrs...)

	return nil
}
