package clientd

import (
	"log/slog"
	"net/http"
)

type Identifier interface {
	MachineID(rebuild bool) string
}

type option struct {
	identifier Identifier
	server     *http.Server
	logger     *slog.Logger
}

func NewOption() *OptionBuilder {
	return &OptionBuilder{}
}

type OptionBuilder struct {
	opts []func(option) option
}

func (b *OptionBuilder) List() []func(option) option {
	return b.opts
}

func (b *OptionBuilder) Logger(l *slog.Logger) *OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.logger = l
		return o
	})
	return b
}

func (b *OptionBuilder) Server(s *http.Server) *OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.server = s
		return o
	})
	return b
}

func (b *OptionBuilder) Handler(h http.Handler) *OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.server = &http.Server{Handler: h}
		return o
	})
	return b
}

func (b *OptionBuilder) Identifier(d Identifier) *OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.identifier = d
		return o
	})
	return b
}

type fallbackOption struct {
}

func (f fallbackOption) List() []func(option) option {
	return []func(option) option{
		func(o option) option {
			if o.server == nil {
				o.server = &http.Server{
					Handler: http.NotFoundHandler(),
				}
			}
			if o.identifier == nil {

			}

			return o
		},
	}
}
