package transport

import (
	"crypto/tls"
	"fmt"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/internal/eventbus"
)

type Config struct {
	Type     string // "local" or "quic"
	Address  string // e.g. "localhost:4242"
	CertFile string
	KeyFile  string
}

func NewServerFromConfig(cfg config.TransportConfig) (eventbus.Transport, error) {
	switch cfg.Type {
	case "local":
		return &LocalTransport{}, nil
	case "quic":
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert: %w", err)
		}
		return NewQUICServer(cfg.Address, cert)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}

func NewClientFromConfig(cfg config.TransportConfig) (eventbus.Transport, error) {
	switch cfg.Type {
	case "local":
		return &LocalTransport{}, nil
	case "quic":
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert: %w", err)
		}
		return NewQUICClient(cfg.Address, &cert)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}
