package tundial

import (
	"context"
	"net"
	"strings"

	"github.com/xmx/aegis-broker/channel/tunnel"
	"github.com/xmx/aegis-common/transport"
	"github.com/xmx/aegis-control/contract/linkhub"
)

func NewDialer(cli tunnel.Client, hub linkhub.Huber, dial ...*net.Dialer) transport.Dialer {
	md := &multiDialer{cli: cli, hub: hub}
	if len(dial) != 0 && dial[0] != nil {
		md.dia = dial[0]
	} else {
		md.dia = new(net.Dialer)
	}

	return md
}

type multiDialer struct {
	cli tunnel.Client
	hub linkhub.Huber
	dia *net.Dialer
}

func (md *multiDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if conn, match, err := md.matchDialTunnel(ctx, address); match {
		return conn, err
	}

	return md.dia.DialContext(ctx, network, address)
}

func (md *multiDialer) matchDialTunnel(ctx context.Context, address string) (net.Conn, bool, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, false, err
	}

	if conn, match, exx := md.matchServer(ctx, host); match {
		return conn, match, exx
	}

	return md.matchAgent(ctx, host)
}

func (md *multiDialer) matchServer(ctx context.Context, address string) (net.Conn, bool, error) {
	if address != transport.ServerHost {
		return nil, false, nil
	}

	mux := md.cli.Muxer()
	if mux == nil {
		return nil, true, &net.AddrError{
			Addr: address,
			Err:  "no route to server host",
		}
	}
	conn, err := mux.Open(ctx)

	return conn, true, err
}

func (md *multiDialer) matchAgent(ctx context.Context, address string) (net.Conn, bool, error) {
	host, found := strings.CutSuffix(address, transport.AgentHostSuffix)
	if !found {
		return nil, false, nil
	}

	peer := md.hub.Get(host)
	if peer == nil {
		return nil, true, &net.AddrError{
			Addr: address,
			Err:  "(broker) no route to agent host",
		}
	}
	mux := peer.Muxer()
	conn, err := mux.Open(ctx)

	return conn, true, err
}
