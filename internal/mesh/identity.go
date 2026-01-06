package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Identity struct {
	ID   string `json:"id"`
	Priv []byte `json:"priv"`
	Pub  []byte `json:"pub"`
}

// loadOrCreateIdentity loads crypto identity from DataDir/identity.json or creates new.
func loadOrCreateIdentity(dataDir string) (*Identity, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("empty DataDir")
	}

	path := filepath.Join(dataDir, "identity.json")

	// Load existing identity if present
	if _, err := os.Stat(path); err == nil {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read identity: %w", err)
		}

		var ident Identity
		if err := json.Unmarshal(b, &ident); err != nil {
			return nil, fmt.Errorf("unmarshal identity: %w", err)
		}

		return &ident, nil
	}

	// Create new identity
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("gen ed25519: %w", err)
	}

	ident := &Identity{
		ID:   hex.EncodeToString(pub),
		Priv: priv,
		Pub:  pub,
	}

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir DataDir: %w", err)
	}

	b, _ := json.MarshalIndent(ident, "", "  ")
	if err := ioutil.WriteFile(path, b, 0644); err != nil {
		return nil, fmt.Errorf("write identity: %w", err)
	}

	return ident, nil
}
