package business

import (
	"context"
	"log/slog"

	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Broker struct {
	repo repository.All
	log  *slog.Logger
}

func NewBroker(repo repository.All, log *slog.Logger) *Broker {
	return &Broker{
		repo: repo,
		log:  log,
	}
}

func (brk *Broker) Get(ctx context.Context, id string) (*model.Broker, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	brkRepo := brk.repo.Broker()

	return brkRepo.FindByID(ctx, oid)
}
