package clientd

import (
	"context"
	"net"
	"sync/atomic"

	"github.com/xmx/aegis-common/tunnel/tunopen"
)

type safeMuxer struct {
	ptr atomic.Pointer[tunopen.Muxer]
}

func (sm *safeMuxer) Accept() (net.Conn, error)                  { return sm.load().Accept() }
func (sm *safeMuxer) Close() error                               { return sm.load().Close() }
func (sm *safeMuxer) Addr() net.Addr                             { return sm.load().Addr() }
func (sm *safeMuxer) Open(ctx context.Context) (net.Conn, error) { return sm.load().Open(ctx) }
func (sm *safeMuxer) RemoteAddr() net.Addr                       { return sm.load().RemoteAddr() }
func (sm *safeMuxer) Protocol() (string, string)                 { return sm.load().Protocol() }
func (sm *safeMuxer) Transferred() (rx, tx uint64)               { return sm.load().Transferred() }

func (sm *safeMuxer) store(mux tunopen.Muxer) {
	if mux == nil {
		panic("通道不能为空")
	}
	if _, ok := mux.(*safeMuxer); ok {
		panic("通道类型错误")
	}

	sm.ptr.Store(&mux)
}

func (sm *safeMuxer) load() tunopen.Muxer {
	if m := sm.ptr.Load(); m != nil {
		return *m
	}

	panic("muxer uninitialized")
}
