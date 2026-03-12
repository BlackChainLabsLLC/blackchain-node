package consensus

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ValidatorKey struct {
	ID         string `json:"id"`          // hex(pubkey)
	PubKeyHex  string `json:"pubkey_hex"`  // hex(pubkey)
	PrivKeyHex string `json:"privkey_hex"` // hex(privkey) - keep file perms tight
}

func LoadOrCreateValidatorKey(dataDir string) (*ValidatorKey, ed25519.PublicKey, ed25519.PrivateKey, error) {
	if dataDir == "" {
		return nil, nil, nil, errors.New("dataDir is empty")
	}
	dir := filepath.Join(dataDir, "consensus")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, nil, err
	}
	path := filepath.Join(dir, "validator_key.json")

	// Load existing
	if b, err := os.ReadFile(path); err == nil {
		var vk ValidatorKey
		if err := json.Unmarshal(b, &vk); err != nil {
			return nil, nil, nil, fmt.Errorf("unmarshal validator key: %w", err)
		}
		pub, err := hex.DecodeString(vk.PubKeyHex)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode pubkey: %w", err)
		}
		priv, err := hex.DecodeString(vk.PrivKeyHex)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode privkey: %w", err)
		}
		if len(pub) != ed25519.PublicKeySize || len(priv) != ed25519.PrivateKeySize {
			return nil, nil, nil, fmt.Errorf("bad key sizes pub=%d priv=%d", len(pub), len(priv))
		}
		return &vk, ed25519.PublicKey(pub), ed25519.PrivateKey(priv), nil
	}

	// Create new
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	pubHex := hex.EncodeToString(pub)
	privHex := hex.EncodeToString(priv)
	vk := ValidatorKey{
		ID:         pubHex,
		PubKeyHex:  pubHex,
		PrivKeyHex: privHex,
	}
	out, err := json.MarshalIndent(&vk, "", "  ")
	if err != nil {
		return nil, nil, nil, err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return nil, nil, nil, err
	}
	return &vk, pub, priv, nil
}
