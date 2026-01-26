package middle

import (
	"github.com/xgfone/ship/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

func NewOtel() ship.Middleware {
	return new(otelMiddle).middle
}

type otelMiddle struct{}

func (om *otelMiddle) middle(h ship.Handler) ship.Handler {
	tracer := otel.Tracer("aegis-server-http")
	return func(c *ship.Context) error {
		req := c.Request()
		parent := otel.GetTextMapPropagator(). // 解析上游 trace
							Extract(req.Context(), propagation.HeaderCarrier(req.Header))

		spanName := req.Method + " " + req.URL.Path
		ctx, span := tracer.Start(parent, spanName)
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", req.Method),
			attribute.String("http.path", req.URL.Path),
		)

		newReq := req.WithContext(ctx)
		defer func() {
			if form := newReq.MultipartForm; form != nil {
				_ = form.RemoveAll()
			}
		}()

		c.SetRequest(newReq)

		return h(c)
	}
}
