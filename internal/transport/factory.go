package transport

import (
	"crypto/tls"
	"fmt"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/internal/eventbus"
	"google.golang.org/protobuf/proto"
)

// Local transport — no constraints on T.
func NewLocal[T any]() eventbus.Transport[T] {
	return &Local[T]{}
}

// QUIC server — T must be a protobuf message.
func NewQUICServerTransport[T proto.Message](addr, certFile, keyFile string) (eventbus.Transport[T], error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert: %w", err)
	}
	return NewQUICServer[T](addr, cert)
}

// QUIC client — T must be a protobuf message.
func NewQUICClientTransport[T proto.Message](addr, certFile, keyFile string) (eventbus.Transport[T], error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert: %w", err)
	}
	return NewQUICClient[T](addr, &cert)
}

// Config-driven helpers (keep signatures similar to your existing usage).
// If cfg.Type == "quic", the caller must instantiate with a protobuf T.

func NewServerFromConfig[T proto.Message](cfg config.TransportConfig) (eventbus.Transport[T], error) {
	switch cfg.Type {
	case "local":
		// Local doesn't actually need T to be proto, but the fn is constrained
		// so it works for both local and quic call sites uniformly.
		return &Local[T]{}, nil
	case "quic":
		return NewQUICServerTransport[T](cfg.Address, cfg.CertFile, cfg.KeyFile)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}

func NewClientFromConfig[T proto.Message](cfg config.TransportConfig) (eventbus.Transport[T], error) {
	switch cfg.Type {
	case "local":
		return &Local[T]{}, nil
	case "quic":
		return NewQUICClientTransport[T](cfg.Address, cfg.CertFile, cfg.KeyFile)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}
