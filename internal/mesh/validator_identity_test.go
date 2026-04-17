package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureValidatorIdentityLockedRejectsInconsistentMaterial(t *testing.T) {
	dir := t.TempDir()
	pubA, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate pubA: %v", err)
	}
	_, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate privB: %v", err)
	}

	raw := `{"pub":"` + hex.EncodeToString(pubA) + `","priv":"` + hex.EncodeToString(privB) + `"}`
	if err := os.WriteFile(filepath.Join(dir, "validator_key.json"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write validator key file: %v", err)
	}

	c := newProductionChain()
	c.persistDir = dir

	if _, err := c.EnsureValidatorIdentityLocked(); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
}

func TestValidatorIDLockedFailsClosedOnBadIdentityMaterial(t *testing.T) {
	dir := t.TempDir()
	raw := `{"pub":"zzz","priv":"bad"}`
	if err := os.WriteFile(filepath.Join(dir, "validator_key.json"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write validator key file: %v", err)
	}

	c := newProductionChain()
	c.persistDir = dir
	if got := c.ValidatorIDLocked(); got != "ERR_NO_VALIDATOR" {
		t.Fatalf("expected fail-closed sentinel, got: %q", got)
	}
}

func TestEnsureValidatorIdentityLockedCreatesValidMaterialWhenMissing(t *testing.T) {
	dir := t.TempDir()
	c := newProductionChain()
	c.persistDir = dir

	pubHex, err := c.EnsureValidatorIdentityLocked()
	if err != nil {
		t.Fatalf("EnsureValidatorIdentityLocked error: %v", err)
	}
	if pubHex == "" || pubHex == "ERR_NO_VALIDATOR" {
		t.Fatalf("unexpected pubHex: %q", pubHex)
	}

	privHex, err := c.ValidatorPrivHexLocked()
	if err != nil {
		t.Fatalf("ValidatorPrivHexLocked error: %v", err)
	}

	if _, _, err := parseValidatorKeyFile(validatorKeyFile{
		PubHex:  pubHex,
		PrivHex: privHex,
	}); err != nil {
		t.Fatalf("generated material failed parse check: %v", err)
	}
}
