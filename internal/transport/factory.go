package transport

import (
	"crypto/tls"
	"fmt"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/internal/eventbus"
	"google.golang.org/protobuf/types/known/anypb"
)

// Local in-proc transport.
func NewLocal() eventbus.Transport[*anypb.Any] {
	return &Local[*anypb.Any]{}
}

// QUIC server transport.
func NewQUICServerTransport(addr, certFile, keyFile string) (eventbus.Transport[*anypb.Any], error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert: %w", err)
	}
	// 👇 removed generic index syntax
	return NewQUICServer(addr, cert)
}

// QUIC client transport.
func NewQUICClientTransport(addr, certFile, keyFile string) (eventbus.Transport[*anypb.Any], error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert: %w", err)
	}
	// 👇 removed generic index syntax
	return NewQUICClient(addr, &cert)
}

// Config-driven server.
func NewServerFromConfig(cfg config.TransportConfig) (eventbus.Transport[*anypb.Any], error) {
	switch cfg.Type {
	case "local":
		return &Local[*anypb.Any]{}, nil
	case "quic":
		return NewQUICServerTransport(cfg.Address, cfg.CertFile, cfg.KeyFile)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}

// Config-driven client.
func NewClientFromConfig(cfg config.TransportConfig) (eventbus.Transport[*anypb.Any], error) {
	switch cfg.Type {
	case "local":
		return &Local[*anypb.Any]{}, nil
	case "quic":
		return NewQUICClientTransport(cfg.Address, cfg.CertFile, cfg.KeyFile)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}
