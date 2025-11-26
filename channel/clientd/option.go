package clientd

import (
	"log/slog"
	"net/http"
)

type option struct {
	server *http.Server
	logger *slog.Logger
}

func NewOption() OptionBuilder {
	return OptionBuilder{}
}

type OptionBuilder struct {
	opts []func(option) option
}

func (b OptionBuilder) List() []func(option) option {
	return b.opts
}

func (b OptionBuilder) Logger(l *slog.Logger) OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.logger = l
		return o
	})
	return b
}

func (b OptionBuilder) Server(s *http.Server) OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.server = s
		return o
	})
	return b
}

func (b OptionBuilder) Handler(h http.Handler) OptionBuilder {
	b.opts = append(b.opts, func(o option) option {
		o.server = &http.Server{Handler: h}
		return o
	})
	return b
}

func fallbackOptions() OptionBuilder {
	return OptionBuilder{
		opts: []func(option) option{
			func(o option) option {
				if o.server == nil {
					srv := &http.Server{Protocols: new(http.Protocols)}
					srv.Protocols.SetUnencryptedHTTP2(true)
					o.server = srv
				}
				if o.server.Handler == nil {
					o.server.Handler = http.NotFoundHandler()
				}

				return o
			},
		},
	}
}
