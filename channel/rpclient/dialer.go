package rpclient

import (
	"context"
	"net"
	"strings"

	"github.com/xmx/aegis-common/muxlink/muxproto"
	"github.com/xmx/aegis-control/linkhub"
)

func NewMixedDialer(mux muxproto.MUXOpener, hub linkhub.Huber, back muxproto.Dialer) muxproto.Dialer {
	return &mixedDialer{
		mux:  mux,
		hub:  hub,
		back: back,
	}
}

type mixedDialer struct {
	mux  muxproto.MUXOpener
	hub  linkhub.Huber
	back muxproto.Dialer
}

func (m *mixedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	if m.mux != nil && host == m.mux.Host() {
		return m.mux.Open(ctx)
	}

	if m.hub != nil {
		_, domain, found := strings.Cut(host, ".")
		if found && domain == m.hub.Domain() {
			peer := m.hub.Get(host)
			if peer == nil {
				return nil, agentUnreachable(network, host)
			}
			mux := peer.Muxer()

			return mux.Open(ctx)
		}
	}

	if m.back != nil {
		return m.back.DialContext(ctx, network, address)
	}

	return nil, &net.OpError{
		Op:   "dial",
		Net:  network,
		Addr: &net.UnixAddr{Net: network, Name: address},
		Err:  net.UnknownNetworkError("没有找到合适的拨号器"),
	}
}

func agentUnreachable(network, address string) error {
	return &net.OpError{
		Op:   "lookup",
		Net:  "agent",
		Addr: &net.UnixAddr{Net: network, Name: address},
		Err:  net.UnknownNetworkError("节点已离线或未注册"),
	}
}
