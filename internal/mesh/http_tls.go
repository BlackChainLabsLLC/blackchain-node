package mesh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// ensureHTTPServerTLSFiles creates/loads a self-signed cert+key pair inside dataDir.
// This is transport encryption (Phase 6.18). Peer authentication remains a separate layer.
func ensureHTTPServerTLSFiles(dataDir, host string) (certPath, keyPath string, _ error) {
	if dataDir == "" {
		return "", "", fmt.Errorf("empty dataDir")
	}
	certPath = filepath.Join(dataDir, "http_tls_cert.pem")
	keyPath = filepath.Join(dataDir, "http_tls_key.pem")

	// If both exist, trust them.
	if _, err := os.Stat(certPath); err == nil {
		if _, err2 := os.Stat(keyPath); err2 == nil {
			return certPath, keyPath, nil
		}
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", "", err
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", "", err
	}

	notBefore := time.Now().Add(-5 * time.Minute)
	notAfter := time.Now().Add(3650 * 24 * time.Hour) // ~10y dev cert

	tpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "blackchain-http",
			Organization: []string{"BlackChain"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add host/IP SANs if we can.
	if ip := net.ParseIP(host); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else if host != "" {
		tpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certOut, err := os.OpenFile(certPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", err
	}
	defer certOut.Close()

	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyOut, err := os.OpenFile(keyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", err
	}
	defer keyOut.Close()

	_ = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPath, keyPath, nil
}

// insecureInternalTLSConfig is used ONLY for internal node-to-node HTTPS calls in local dev.
// It provides encryption (privacy) without strict identity verification at this layer.
func insecureInternalTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // transport encryption only; auth handled elsewhere
		MinVersion:         tls.VersionTLS12,
	}
}
