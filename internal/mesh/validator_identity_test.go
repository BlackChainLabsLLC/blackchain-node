package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

func TestEnsureValidatorIdentityLockedRejectsMalformedKeyFile(t *testing.T) {
	c := newTestChain(t)
	path := c.validatorKeyPathLocked()
	if err := os.WriteFile(path, []byte(`{"pub":"bad","priv":"bad"}`), 0o600); err != nil {
		t.Fatalf("write validator key file: %v", err)
	}

	if _, err := c.EnsureValidatorIdentityLocked(); err == nil {
		t.Fatalf("expected malformed validator key rejection")
	}
}

func TestEnsureValidatorIdentityLockedRejectsPubPrivMismatch(t *testing.T) {
	c := newTestChain(t)
	pubA, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate pubA: %v", err)
	}
	_, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate privB: %v", err)
	}
	raw := []byte(`{"pub":"` + hex.EncodeToString(pubA) + `","priv":"` + hex.EncodeToString(privB) + `"}`)
	if err := os.WriteFile(c.validatorKeyPathLocked(), raw, 0o600); err != nil {
		t.Fatalf("write validator key file: %v", err)
	}

	if _, err := c.EnsureValidatorIdentityLocked(); err == nil {
		t.Fatalf("expected pub/priv mismatch rejection")
	}
}

func TestRequireValidatorActionReadyLockedRejectsMisuse(t *testing.T) {
	c := newTestChain(t)
	if err := c.requireValidatorActionReadyLocked("node2"); err == nil {
		t.Fatalf("expected non-leader validator action rejection")
	}

	c.validatorPubHex = "bad"
	c.validatorPrivHex = "bad"
	if err := c.requireValidatorActionReadyLocked("node1"); err == nil {
		t.Fatalf("expected bad cached validator identity rejection")
	}
}
