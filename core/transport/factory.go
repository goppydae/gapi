package transport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/eventbus"
	"google.golang.org/protobuf/types/known/anypb"
)

// Local in-proc transport.
func NewLocal() eventbus.Transport[*anypb.Any] {
	return &Local[*anypb.Any]{}
}

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"GAPI"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}, nil
}

// QUIC server transport.
func NewQUICServerTransport(addr, tlsCert, tlsKey string) (eventbus.Transport[*anypb.Any], error) {
	var cert tls.Certificate
	var err error

	if tlsCert == "" || tlsKey == "" {
		cert, err = generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("generate self-signed cert: %w", err)
		}
	} else {
		cert, err = tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return nil, fmt.Errorf("load cert: %w", err)
		}
	}
	// 👇 removed generic index syntax
	return NewQUICServer(addr, cert)
}

// QUIC client transport.
func NewQUICClientTransport(cfg config.TransportConfig) (eventbus.Transport[*anypb.Any], error) {
	var cert *tls.Certificate

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		c, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("load cert: %w", err)
		}
		cert = &c
	}
	tlsCfg := TLSConfig{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		CAFile:             cfg.TLSCA,
	}
	return NewQUICClient(cfg.Address, cert, tlsCfg)
}

// Config-driven server.
func NewServerFromConfig(cfg config.TransportConfig) (eventbus.Transport[*anypb.Any], error) {
	switch cfg.Type {
	case "local":
		return &Local[*anypb.Any]{}, nil
	case "quic":
		return NewQUICServerTransport(cfg.Address, cfg.TLSCert, cfg.TLSKey)
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
		return NewQUICClientTransport(cfg)
	default:
		return nil, fmt.Errorf("unknown transport type: %s", cfg.Type)
	}
}
