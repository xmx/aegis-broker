package serverd

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/xmx/aegis-common/validation"
	"github.com/xmx/aegis-control/linkhub"
	"golang.org/x/time/rate"
)

type option struct {
	server *http.Server
	logger *slog.Logger
	huber  linkhub.Huber
	limit  *rate.Limiter
	valid  *validation.Validate
	parent context.Context
}

func NewOption() OptionBuilder {
	return OptionBuilder{}
}

type OptionBuilder struct {
	opts []func(option) option
}

func (ob OptionBuilder) List() []func(option) option {
	return ob.opts
}

func (ob OptionBuilder) Logger(log *slog.Logger) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.logger = log
		return o
	})
	return ob
}

func (ob OptionBuilder) Server(s *http.Server) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.server = s
		return o
	})

	return ob
}

func (ob OptionBuilder) Handler(h http.Handler) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		if o.server == nil {
			o.server = &http.Server{}
		}
		o.server.Handler = h

		return o
	})
	return ob
}

func (ob OptionBuilder) Validator(v *validation.Validate) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.valid = v
		return o
	})
	return ob
}

func (ob OptionBuilder) Limiter(l *rate.Limiter) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.limit = l
		return o
	})
	return ob
}

func (ob OptionBuilder) Huber(h linkhub.Huber) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.huber = h
		return o
	})
	return ob
}
