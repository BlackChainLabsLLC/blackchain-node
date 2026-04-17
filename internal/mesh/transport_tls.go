package mesh

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	if t.CertFile == "" || t.KeyFile == "" || t.CAFile == "" {
		return nil, fmt.Errorf("tls enabled but cert/key/ca not fully configured")
	}
	if err := validateTLSPaths(t); err != nil {
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

func validateTLSPaths(t *MeshTLS) error {
	if t == nil {
		return nil
	}
	if err := validateTLSFile("tls.cert_file", t.CertFile, false); err != nil {
		return err
	}
	if err := validateTLSFile("tls.key_file", t.KeyFile, true); err != nil {
		return err
	}
	if err := validateTLSFile("tls.ca_file", t.CAFile, false); err != nil {
		return err
	}
	return nil
}

func validateTLSFile(field, path string, key bool) error {
	if path == "" {
		return fmt.Errorf("%s is empty", field)
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s not accessible (%s): %w", field, path, err)
	}
	if st.IsDir() {
		return fmt.Errorf("%s must be a file, got directory: %s", field, path)
	}
	if key {
		mode := st.Mode().Perm()
		if mode&0o077 != 0 {
			return fmt.Errorf("%s (%s) is too permissive (%#o); expected owner-only permissions", field, path, mode)
		}
	}
	clean := filepath.Clean(path)
	f, err := os.Open(clean)
	if err != nil {
		return fmt.Errorf("%s not readable (%s): %w", field, path, err)
	}
	_ = f.Close()
	return nil
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
