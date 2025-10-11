package service

import (
	"context"
	"log/slog"

	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func NewSystem(repo repository.All, log *slog.Logger) *System {
	return &System{
		repo: repo,
		log:  log,
	}
}

type System struct {
	repo repository.All
	log  *slog.Logger
}

func (s *System) Network(ctx context.Context, req model.NodeNetworks, p linkhub.Peer) error {
	update := bson.M{"$set": bson.M{"networks": req}}
	repo := s.repo.Agent()
	_, err := repo.UpdateByID(ctx, p.ID(), update)

	return err
}
