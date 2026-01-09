package serverd

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/xmx/aegis-common/muxlink/muxconn"
	"github.com/xmx/aegis-control/linkhub"
)

type Options struct {
	Handler http.Handler
	Huber   linkhub.Huber
	Logger  *slog.Logger
	Valid   func(*AuthRequest) error
	Allowed func(muxconn.Muxer) bool
	Timeout time.Duration
	Context context.Context
}

func (o Options) log() *slog.Logger {
	if l := o.Logger; l != nil {
		return l
	}

	return slog.Default()
}

func (o Options) allowed(mux muxconn.Muxer) bool {
	if f := o.Allowed; f != nil {
		return f(mux)
	}
	return true
}

func (o Options) valid(v *AuthRequest) error {
	if f := o.Valid; f != nil {
		return f(v)
	}

	return nil
}
