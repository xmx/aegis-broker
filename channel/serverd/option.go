package serverd

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/xmx/aegis-common/muxlink/muxconn"
	"github.com/xmx/aegis-control/linkhub"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Options struct {
	CurrentBroker CurrentBroker
	ServerHooker  linkhub.ServerHooker
	Handler       http.Handler
	Huber         linkhub.Huber
	Validator     func(any) error // 认证报文参数校验器
	Limiter       func(muxconn.Muxer) bool
	Logger        *slog.Logger
	Timeout       time.Duration
	Context       context.Context
}

type CurrentBroker struct {
	ID   bson.ObjectID
	Name string
}
