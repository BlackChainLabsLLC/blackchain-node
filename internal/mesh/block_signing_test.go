package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestValidateBlockSignatureRejectsValidatorIdentityMismatch(t *testing.T) {
	c, signer := newTestChainWithSigner(t)
	b := makeSignedBlock(t, c, signer, 1, "", time.Now().UTC())
	b.ValidatorID = strings.Repeat("1", ed25519.PublicKeySize*2)

	err := ValidateBlockSignature(b)
	if err == nil || !strings.Contains(err.Error(), "validator_id/producer mismatch") {
		t.Fatalf("expected validator identity mismatch rejection, got %v", err)
	}
}

func TestValidateBlockSignatureRejectsTamperedSignature(t *testing.T) {
	c, signer := newTestChainWithSigner(t)
	b := makeSignedBlock(t, c, signer, 1, "", time.Now().UTC())
	b.Signature = strings.Repeat("0", ed25519.SignatureSize*2)

	err := ValidateBlockSignature(b)
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected tampered signature rejection, got %v", err)
	}
}

func TestApplyBlockOrBufferRejectsInvalidSignerIdentity(t *testing.T) {
	c, signer := newTestChainWithSigner(t)
	b := makeSignedBlock(t, c, signer, 1, "", time.Now().UTC())
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other pub: %v", err)
	}
	b.Producer = hex.EncodeToString(otherPub)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, signer.priv)

	_, err = c.applyBlockOrBufferLocked(b)
	if err == nil || !strings.Contains(err.Error(), "invalid block signature") {
		t.Fatalf("expected invalid signer identity rejection, got %v", err)
	}
}
