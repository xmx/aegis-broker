package errcode

import (
	"net/http"

	"github.com/xgfone/ship/v5"
)

var FmtAgentDisconnect = errorTemplate("Agent 节点离线：%s")

type errorTemplate string

func (et errorTemplate) Fmt(args ...any) ship.HTTPServerError {
	return et.WithCode(http.StatusBadRequest, args...)
}

func (et errorTemplate) WithCode(code int, args ...any) ship.HTTPServerError {
	return ship.NewHTTPServerError(code).Newf(string(et), args...)
}
