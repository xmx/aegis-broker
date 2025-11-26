package serverd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/xmx/aegis-control/linkhub"
)

type option struct {
	server  *http.Server
	logger  *slog.Logger
	huber   linkhub.Huber
	allow   func() bool
	valid   func(req any) error
	parent  context.Context
	timeout time.Duration
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

func (ob OptionBuilder) Valid(v func(any) error) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.valid = v
		return o
	})
	return ob
}

func (ob OptionBuilder) Allow(v func() bool) OptionBuilder {
	ob.opts = append(ob.opts, func(o option) option {
		o.allow = v
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

func defaultValid(v any) error {
	req, ok := v.(*authRequest)
	if !ok {
		return errors.New("无效的认证结构体类型")
	}
	if req.MachineID == "" {
		return errors.New("机器码不能为空")
	}
	if req.Inet == "" {
		return errors.New("IP 不能为空")
	}
	if req.Goos == "" {
		return errors.New("系统类型不能为空")
	}
	if req.Goarch == "" {
		return errors.New("系统类型不能为空")
	}

	return nil
}

func fallbackOptions() OptionBuilder {
	return OptionBuilder{
		opts: []func(option) option{
			func(o option) option {
				if o.parent == nil {
					o.parent = context.Background()
				}
				if o.server == nil {
					srv := &http.Server{Protocols: new(http.Protocols)}
					srv.Protocols.SetUnencryptedHTTP2(true)
					o.server = srv
				}
				if o.server.Handler == nil {
					o.server.Handler = http.NotFoundHandler()
				}
				if o.huber == nil {
					o.huber = linkhub.NewHub(1024)
				}
				if o.valid == nil {
					o.valid = defaultValid
				}
				if o.allow == nil {
					o.allow = func() bool { return true }
				}
				return o
			},
		},
	}
}
