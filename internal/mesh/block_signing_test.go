package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func makeSignedBlockForTest(t *testing.T) (Block, ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate validator key: %v", err)
	}
	pubHex := hex.EncodeToString(pub)

	b := Block{
		Producer:     pubHex,
		ValidatorID:  pubHex,
		Height:       1,
		PrevHash:     "",
		TimeUTC:      time.Unix(100, 0).UTC(),
		BalancesRoot: "root",
	}
	c := &ProductionChain{}
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, priv)
	return b, priv, pub
}

func TestValidateBlockSignatureRejectsUnsignedBlock(t *testing.T) {
	b, _, _ := makeSignedBlockForTest(t)
	b.Signature = ""
	if err := ValidateBlockSignature(b); err == nil || !strings.Contains(err.Error(), "missing signature") {
		t.Fatalf("expected missing signature error, got: %v", err)
	}
}

func TestValidateBlockSignatureRejectsValidatorMismatch(t *testing.T) {
	b, _, _ := makeSignedBlockForTest(t)
	b.ValidatorID = strings.Repeat("a", ed25519.PublicKeySize*2)
	if err := ValidateBlockSignature(b); err == nil || !strings.Contains(err.Error(), "validator identity mismatch") {
		t.Fatalf("expected validator mismatch error, got: %v", err)
	}
}

func TestValidateBlockSignatureRejectsInvalidSignerIdentity(t *testing.T) {
	b, _, _ := makeSignedBlockForTest(t)
	b.Producer = "abcd"
	if err := ValidateBlockSignature(b); err == nil || !strings.Contains(err.Error(), "validator identity mismatch") {
		t.Fatalf("expected validator mismatch due to invalid signer identity, got: %v", err)
	}
}

func TestValidateBlockSignatureRejectsTamperedSignature(t *testing.T) {
	b, _, _ := makeSignedBlockForTest(t)
	b.TimeUTC = b.TimeUTC.Add(time.Second)
	if err := ValidateBlockSignature(b); err == nil || !strings.Contains(err.Error(), "invalid block signature") {
		t.Fatalf("expected invalid signature error, got: %v", err)
	}
}
