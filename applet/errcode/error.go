package errcode

import "github.com/xgfone/ship/v5"

var (
	ErrNilDocument            = ship.ErrBadRequest.Newf("数据不存在")
	ErrCertificateInvalid     = ship.ErrBadRequest.Newf("无效证书")
	ErrCertificateUnavailable = ship.ErrBadRequest.Newf("未配置有效的证书")
)
