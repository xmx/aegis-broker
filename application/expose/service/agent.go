package service

import (
	"context"
	"log/slog"

	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Agent struct {
	repo repository.All
	log  *slog.Logger
}

func NewAgent(repo repository.All, log *slog.Logger) *Agent {
	return &Agent{
		repo: repo,
		log:  log,
	}
}

func (agt *Agent) Reset(ctx context.Context, brokerID bson.ObjectID) error {
	filter := bson.M{"broker.id": brokerID, "status": true}
	update := bson.M{"$set": bson.M{"status": false}}
	repo := agt.repo.Agent()
	_, err := repo.UpdateMany(ctx, filter, update)

	return err
}
