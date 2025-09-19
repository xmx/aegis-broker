package service

import (
	"context"
	"log/slog"

	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Agent struct {
	this *model.Broker
	repo repository.All
	log  *slog.Logger
}

func NewAgent(this *model.Broker, repo repository.All, log *slog.Logger) *Agent {
	return &Agent{
		this: this,
		repo: repo,
		log:  log,
	}
}

func (agt *Agent) Reset(ctx context.Context) error {
	filter := bson.M{"broker.id": agt.this.ID, "status": true}
	update := bson.M{"$set": bson.M{"status": false}}
	repo := agt.repo.Agent()
	_, err := repo.UpdateMany(ctx, filter, update)

	return err
}
