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

func parseValidatorKeyFile(raw []byte) (validatorKeyFile, error) {
	var k validatorKeyFile
	if len(raw) == 0 {
		return k, fmt.Errorf("validator key file is empty")
	}
	if err := json.Unmarshal(raw, &k); err != nil {
		return k, fmt.Errorf("decode validator key file: %w", err)
	}
	if k.PubHex == "" || k.PrivHex == "" {
		return k, fmt.Errorf("validator key file must contain pub and priv")
	}
	pubRaw, err := hex.DecodeString(k.PubHex)
	if err != nil {
		return k, fmt.Errorf("validator pub hex: %w", err)
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return k, fmt.Errorf("validator pub key size %d != %d", len(pubRaw), ed25519.PublicKeySize)
	}
	privRaw, err := hex.DecodeString(k.PrivHex)
	if err != nil {
		return k, fmt.Errorf("validator priv hex: %w", err)
	}
	if len(privRaw) != ed25519.PrivateKeySize {
		return k, fmt.Errorf("validator priv key size %d != %d", len(privRaw), ed25519.PrivateKeySize)
	}
	derivedPub := ed25519.PrivateKey(privRaw).Public().(ed25519.PublicKey)
	if !ed25519.PublicKey(pubRaw).Equal(derivedPub) {
		return k, fmt.Errorf("validator key file pub/priv mismatch")
	}
	return k, nil
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

func (c *ProductionChain) EnsureValidatorIdentityLocked() (string, error) {
	if c.validatorPubHex != "" && c.validatorPrivHex != "" {
		pubRaw, err := hex.DecodeString(c.validatorPubHex)
		if err != nil || len(pubRaw) != ed25519.PublicKeySize {
			return "", fmt.Errorf("cached validator pub hex invalid")
		}
		privRaw, err := hex.DecodeString(c.validatorPrivHex)
		if err != nil || len(privRaw) != ed25519.PrivateKeySize {
			return "", fmt.Errorf("cached validator priv hex invalid")
		}
		derivedPub := ed25519.PrivateKey(privRaw).Public().(ed25519.PublicKey)
		if !ed25519.PublicKey(pubRaw).Equal(derivedPub) {
			return "", fmt.Errorf("cached validator pub/priv mismatch")
		}
		return c.validatorPubHex, nil
	}
	p := c.validatorKeyPathLocked()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)

	// Try load existing
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		k, err := parseValidatorKeyFile(b)
		if err != nil {
			return "", err
		}
		c.validatorPubHex = k.PubHex
		c.validatorPrivHex = k.PrivHex
		return c.validatorPubHex, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read validator key file: %w", err)
	}

	// Create new
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate validator keypair: %w", err)
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
		return "", fmt.Errorf("write validator key file: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", fmt.Errorf("rename validator key file: %w", err)
	}

	c.validatorPubHex = k.PubHex
	c.validatorPrivHex = k.PrivHex
	return c.validatorPubHex, nil
}

// ValidatorIDLocked returns the validator public key hex for this node.
// Caller must hold c.mu (write or read is fine if we only lazily set fields).
func (c *ProductionChain) ValidatorIDLocked() string {
	pub, err := c.EnsureValidatorIdentityLocked()
	if err != nil {
		c.validatorPubHex = "ERR_NO_VALIDATOR"
		c.validatorPrivHex = ""
		return c.validatorPubHex
	}
	return pub
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
	if _, err := c.EnsureValidatorIdentityLocked(); err != nil {
		return "", err
	}
	return c.validatorPrivHex, nil
}
