package mesh

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MeshTLS holds mTLS configuration for mesh TCP transport.
// tlsConfigFrom builds a tls.Config suitable for both server and client.
// - Server: requires & verifies client certs (mTLS).
// - Client: verifies server cert against CA.
func tlsConfigFrom(t *MeshTLS, isServer bool) (*tls.Config, error) {
	if t == nil || !t.Enabled {
		return nil, nil
	}
	if err := t.validate(); err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}

	caPEM, err := os.ReadFile(t.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("append ca pem: failed")
	}

	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool, // used by client
		ClientCAs:    pool, // used by server
	}

	if isServer {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}

func meshListen(listenAddr string, t *MeshTLS) (net.Listener, error) {
	if t == nil || !t.Enabled {
		return net.Listen("tcp", listenAddr)
	}
	tc, err := tlsConfigFrom(t, true)
	if err != nil {
		return nil, err
	}
	return tls.Listen("tcp", listenAddr, tc)
}

func meshDialTimeout(ctx context.Context, addr string, timeout time.Duration, t *MeshTLS) (net.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	if t == nil || !t.Enabled {
		return d.DialContext(ctx, "tcp", addr)
	}
	tc, err := tlsConfigFrom(t, false)
	if err != nil {
		return nil, err
	}
	return tls.DialWithDialer(d, "tcp", addr, tc)
}

func (t *MeshTLS) validate() error {
	if t == nil || !t.Enabled {
		return nil
	}
	if t.CertFile == "" || t.KeyFile == "" || t.CAFile == "" {
		return fmt.Errorf("config validation: tls enabled but cert_file/key_file/ca_file are not fully configured")
	}
	if err := validateTLSFile("tls cert_file", t.CertFile, false); err != nil {
		return err
	}
	if err := validateTLSFile("tls key_file", t.KeyFile, true); err != nil {
		return err
	}
	if err := validateTLSFile("tls ca_file", t.CAFile, false); err != nil {
		return err
	}

	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return fmt.Errorf("config validation: tls cert_file/key_file mismatch: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("config validation: tls cert_file does not contain a certificate")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("config validation: parse tls cert_file: %w", err)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("config validation: tls cert_file is not valid before %s", leaf.NotBefore.UTC().Format(time.RFC3339))
	}
	if now.After(leaf.NotAfter) {
		return fmt.Errorf("config validation: tls cert_file expired at %s", leaf.NotAfter.UTC().Format(time.RFC3339))
	}

	caPEM, err := os.ReadFile(t.CAFile)
	if err != nil {
		return fmt.Errorf("config validation: read tls ca_file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("config validation: tls ca_file does not contain valid PEM certificates")
	}
	return nil
}

func validateTLSFile(label, path string, privateKey bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("config validation: %s must not be empty", label)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config validation: %s not accessible at %s: %w", label, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("config validation: %s path must be a file, not a directory: %s", label, path)
	}
	if _, err := os.ReadFile(path); err != nil {
		return fmt.Errorf("config validation: %s is not readable at %s: %w", label, path, err)
	}
	if privateKey && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config validation: %s permissions are too broad on %s; require owner-only access", label, filepath.Clean(path))
	}
	return nil
}
