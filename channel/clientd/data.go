package clientd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/xmx/aegis-common/muxlink/muxconn"
)

type authRequest struct {
	Secret     string   `json:"secret"`
	Semver     string   `json:"semver"`
	Inet       string   `json:"inet"`
	Goos       string   `json:"goos"`
	Goarch     string   `json:"goarch"`
	PID        int      `json:"pid,omitzero"`
	Args       []string `json:"args,omitzero"`
	Hostname   string   `json:"hostname,omitzero"`
	Workdir    string   `json:"workdir,omitzero"`
	Executable string   `json:"executable,omitzero"`
}

type authResponse struct {
	Code    int         `json:"code"`    // 状态码
	Message string      `json:"message"` // 错误信息
	Config  *AuthConfig `json:"config"`
}

type AuthConfig struct {
	URI string `json:"uri"` // mongo 连接
}

func (ar authResponse) checkError() error {
	code := ar.Code
	if code >= http.StatusOK && code < http.StatusMultipleChoices {
		return nil
	}

	return fmt.Errorf("通道上线失败 %d: %s", ar.Code, ar.Message)
}

type muxInstance struct {
	ptr atomic.Pointer[muxconn.Muxer]
}

func (m *muxInstance) Accept() (net.Conn, error)                  { return m.load().Accept() }
func (m *muxInstance) Close() error                               { return m.load().Close() }
func (m *muxInstance) Addr() net.Addr                             { return m.load().Addr() }
func (m *muxInstance) Open(ctx context.Context) (net.Conn, error) { return m.load().Open(ctx) }
func (m *muxInstance) RemoteAddr() net.Addr                       { return m.load().RemoteAddr() }
func (m *muxInstance) Protocol() (string, string)                 { return m.load().Protocol() }
func (m *muxInstance) Traffic() (rx, tx uint64)                   { return m.load().Traffic() }
func (m *muxInstance) load() muxconn.Muxer                        { return *m.ptr.Load() }
func (m *muxInstance) store(mux muxconn.Muxer)                    { m.ptr.Store(&mux) }
