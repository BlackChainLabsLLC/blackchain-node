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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ensureHTTPServerTLSFiles returns HTTP TLS material.
// - If mesh TLS is configured, reuse that CA-issued keypair.
// - Otherwise, use a shared local HTTP CA rooted under the data dir parent.
func ensureHTTPServerTLSFiles(dataDir, host string, meshTLS *MeshTLS) (certPath, keyPath string, _ error) {
	if meshTLS != nil && meshTLS.Enabled && meshTLS.CertFile != "" && meshTLS.KeyFile != "" && meshTLS.CAFile != "" {
		return meshTLS.CertFile, meshTLS.KeyFile, nil
	}
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

	caCertPath, caKeyPath := sharedHTTPCAPaths(dataDir)
	caCert, caKey, err := loadOrCreateSharedHTTPCA(caCertPath, caKeyPath)
	if err != nil {
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
	notAfter := time.Now().Add(3650 * 24 * time.Hour)

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

	if ip := net.ParseIP(host); ip != nil {
		tpl.IPAddresses = []net.IP{ip}
	} else if host != "" {
		tpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tpl, caCert, &priv.PublicKey, caKey)
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

// insecureInternalTLSConfig is allowed only for explicit dev/lab use.
func insecureInternalTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}
}

func newInternalTLSConfig(dataDir string, meshTLS *MeshTLS) (*tls.Config, error) {
	if allowDevInsecureHTTP() {
		return insecureInternalTLSConfig(), nil
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}

	switch {
	case meshTLS != nil && meshTLS.Enabled && meshTLS.CAFile != "":
		caPEM, err := os.ReadFile(meshTLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read mesh http ca: %w", err)
		}
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("append mesh http ca pem: failed")
		}
	case dataDir != "":
		caCertPath, caKeyPath := sharedHTTPCAPaths(dataDir)
		if _, _, err := loadOrCreateSharedHTTPCA(caCertPath, caKeyPath); err != nil {
			return nil, fmt.Errorf("load shared http ca: %w", err)
		}
		caPEM, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("read shared http ca: %w", err)
		}
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("append shared http ca pem: failed")
		}
	default:
		return nil, fmt.Errorf("missing trust roots for internal https client")
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}, nil
}

func newInternalHTTPSClient(timeout time.Duration, dataDir string, meshTLS *MeshTLS) (*http.Client, error) {
	tlsCfg, err := newInternalTLSConfig(dataDir, meshTLS)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		TLSClientConfig: tlsCfg,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}, nil
}

func InsecureBlackctlTLSConfig() *tls.Config {
	if allowDevInsecureHTTP() {
		return insecureInternalTLSConfig()
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, path := range []string{
		strings.TrimSpace(os.Getenv("BLACKCTL_CA_FILE")),
		filepath.Join("data", "http_ca.pem"),
	} {
		if path == "" {
			continue
		}
		if caPEM, err := os.ReadFile(path); err == nil {
			if pool.AppendCertsFromPEM(caPEM) {
				break
			}
		}
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
}

func allowDevInsecureHTTP() bool {
	v := strings.TrimSpace(os.Getenv("BLACKCHAIN_DEV_INSECURE_HTTP"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func sharedHTTPCAPaths(dataDir string) (certPath, keyPath string) {
	base := filepath.Dir(filepath.Clean(dataDir))
	return filepath.Join(base, "http_ca.pem"), filepath.Join(base, "http_ca.key")
}

func loadOrCreateSharedHTTPCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	if certPEM, err := os.ReadFile(certPath); err == nil {
		if keyPEM, err2 := os.ReadFile(keyPath); err2 == nil {
			certBlock, _ := pem.Decode(certPEM)
			keyBlock, _ := pem.Decode(keyPEM)
			if certBlock != nil && keyBlock != nil {
				cert, err := x509.ParseCertificate(certBlock.Bytes)
				if err == nil {
					key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
					if err == nil {
						return cert, key, nil
					}
				}
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return nil, nil, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, nil, err
	}
	notBefore := time.Now().Add(-5 * time.Minute)
	notAfter := time.Now().Add(3650 * 24 * time.Hour)
	tpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "blackchain-http-ca",
			Organization: []string{"BlackChain"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}
