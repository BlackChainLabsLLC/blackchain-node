package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type NodeKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func LoadOrCreateNodeKeys(dataDir string) (*NodeKeys, error) {
	path := filepath.Join(dataDir, "nodekey.json")

	// load existing
	if b, err := os.ReadFile(path); err == nil {
		var k NodeKeys
		if err := json.Unmarshal(b, &k); err != nil {
			return nil, err
		}
		return &k, nil
	}

	// generate
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	k := NodeKeys{
		PublicKey:  hex.EncodeToString(pub),
		PrivateKey: hex.EncodeToString(priv),
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}

	b, _ := json.MarshalIndent(k, "", "  ")
	if err := os.WriteFile(path, b, 0600); err != nil {
		return nil, err
	}

	return &k, nil
}

func DecodePrivateKey(hexkey string) (ed25519.PrivateKey, error) {
	raw, err := hex.DecodeString(hexkey)
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(raw), nil
}

func DecodePublicKey(hexkey string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(hexkey)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(raw), nil
}
