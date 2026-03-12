package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Phase 3.1: Validator Identity
// - Each node has a persistent ed25519 keypair stored under its chain persist/data dir.
// - We expose ValidatorID (pubkey hex) to stamp blocks.
// - This is identity only (no votes yet).

type validatorKeyFile struct {
	PubHex  string `json:"pub"`
	PrivHex string `json:"priv"` // ed25519.PrivateKey (64 bytes) hex
}

func (c *ProductionChain) validatorKeyPathLocked() string {
	// prefer persistDir (stable), else dataDir, else local "data"
	base := stringsTrim(c.persistDir)
	if base == "" {
		base = stringsTrim(c.dataDir)
	}
	if base == "" {
		base = "data"
	}
	return filepath.Join(base, "validator_key.json")
}

// ValidatorIDLocked returns the validator public key hex for this node.
// Caller must hold c.mu (write or read is fine if we only lazily set fields).
func (c *ProductionChain) ValidatorIDLocked() string {
	// cache in-memory
	if c.validatorPubHex != "" && c.validatorPrivHex != "" {
		return c.validatorPubHex
	}

	p := c.validatorKeyPathLocked()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)

	// Try load existing
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		var k validatorKeyFile
		if err := json.Unmarshal(b, &k); err == nil && k.PubHex != "" && k.PrivHex != "" {
			c.validatorPubHex = k.PubHex
			c.validatorPrivHex = k.PrivHex
			return c.validatorPubHex
		}
	}

	// Create new
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		// last resort: keep chain functional but explicit error
		c.validatorPubHex = "ERR_NO_VALIDATOR"
		c.validatorPrivHex = ""
		return c.validatorPubHex
	}

	k := validatorKeyFile{
		PubHex:  hex.EncodeToString(pub),
		PrivHex: hex.EncodeToString(priv),
	}
	raw, _ := json.MarshalIndent(k, "", "  ")
	raw = append(raw, '\n')

	// write atomically
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		c.validatorPubHex = "ERR_NO_VALIDATOR"
		c.validatorPrivHex = ""
		return c.validatorPubHex
	}
	_ = os.Rename(tmp, p)

	c.validatorPubHex = k.PubHex
	c.validatorPrivHex = k.PrivHex
	return c.validatorPubHex
}

// tiny helper: avoid importing strings everywhere in core file
func stringsTrim(s string) string {
	// minimal trim of spaces and trailing slashes
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		last := s[len(s)-1]
		if last == ' ' || last == '\t' || last == '\n' || last == '\r' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	if s == "" {
		return ""
	}
	// trim trailing slash
	for len(s) > 1 && (s[len(s)-1] == '/') {
		s = s[:len(s)-1]
	}
	return s
}

func (c *ProductionChain) ValidatorPrivHexLocked() (string, error) {
	if c.validatorPrivHex == "" {
		_ = c.ValidatorIDLocked()
	}
	if c.validatorPrivHex == "" {
		return "", fmt.Errorf("missing validator private key")
	}
	return c.validatorPrivHex, nil
}
