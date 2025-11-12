package restapi

import (
	"log/slog"
	"net/http"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/business"
)

type Certificate struct {
	biz *business.Certificate
	log *slog.Logger
}

func NewCertificate(biz *business.Certificate, log *slog.Logger) *Certificate {
	return &Certificate{
		biz: biz,
		log: log,
	}
}

func (crt *Certificate) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/certificate/forget").DELETE(crt.forget)
	return nil
}

func (crt *Certificate) forget(c *ship.Context) error {
	crt.biz.Forget()
	c.Warnf("清除证书缓存")

	return c.NoContent(http.StatusNoContent)
}
