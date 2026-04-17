package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Phase 3.1: Validator Identity
// - Each node has a persistent ed25519 keypair stored under its chain persist/data dir.
// - We expose ValidatorID (pubkey hex) to stamp blocks.
// - This is identity only (no votes yet).

type validatorKeyFile struct {
	PubHex  string `json:"pub"`
	PrivHex string `json:"priv"` // ed25519.PrivateKey (64 bytes) hex
}

func parseValidatorKeyFile(k validatorKeyFile) (string, string, error) {
	pubHex := strings.ToLower(stringsTrim(k.PubHex))
	privHex := strings.ToLower(stringsTrim(k.PrivHex))

	if pubHex == "" || privHex == "" {
		return "", "", fmt.Errorf("validator key file must include both pub and priv")
	}

	pubRaw, err := hex.DecodeString(pubHex)
	if err != nil {
		return "", "", fmt.Errorf("bad validator pub hex: %w", err)
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return "", "", fmt.Errorf("bad validator pub length: got=%d want=%d", len(pubRaw), ed25519.PublicKeySize)
	}

	privRaw, err := hex.DecodeString(privHex)
	if err != nil {
		return "", "", fmt.Errorf("bad validator priv hex: %w", err)
	}
	if len(privRaw) != ed25519.PrivateKeySize {
		return "", "", fmt.Errorf("bad validator priv length: got=%d want=%d", len(privRaw), ed25519.PrivateKeySize)
	}

	privPub := ed25519.PrivateKey(privRaw).Public()
	derivedPub, ok := privPub.(ed25519.PublicKey)
	if !ok || len(derivedPub) != ed25519.PublicKeySize {
		return "", "", fmt.Errorf("validator private key cannot derive public identity")
	}
	if !ed25519.PublicKey(pubRaw).Equal(derivedPub) {
		return "", "", fmt.Errorf("validator key mismatch: pub does not match priv")
	}

	return pubHex, privHex, nil
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

	pubHex, privHex, err := c.ensureValidatorIdentityLocked()
	if err != nil {
		c.validatorPubHex = "ERR_NO_VALIDATOR"
		c.validatorPrivHex = ""
		return c.validatorPubHex
	}
	c.validatorPubHex = pubHex
	c.validatorPrivHex = privHex
	return c.validatorPubHex
}

func (c *ProductionChain) ensureValidatorIdentityLocked() (string, string, error) {
	// cache in-memory if already validated
	if c.validatorPubHex != "" && c.validatorPrivHex != "" {
		return c.validatorPubHex, c.validatorPrivHex, nil
	}

	p := c.validatorKeyPathLocked()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)

	// Try load existing
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		var k validatorKeyFile
		if err := json.Unmarshal(b, &k); err != nil {
			return "", "", fmt.Errorf("validator key file is not valid json: %w", err)
		}
		pubHex, privHex, err := parseValidatorKeyFile(k)
		if err != nil {
			return "", "", err
		}
		return pubHex, privHex, nil
	}

	// Create new
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate validator keypair: %w", err)
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
		return "", "", fmt.Errorf("write validator key file: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", "", fmt.Errorf("persist validator key file: %w", err)
	}

	return k.PubHex, k.PrivHex, nil
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

func (c *ProductionChain) EnsureValidatorIdentityLocked() (string, error) {
	pubHex, privHex, err := c.ensureValidatorIdentityLocked()
	if err != nil {
		return "", err
	}
	c.validatorPubHex = pubHex
	c.validatorPrivHex = privHex
	return pubHex, nil
}
