package mesh

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

// blockSignHash returns the canonical hash bytes to be signed.
// It MUST match chain block hashing.
func blockSignHash(b Block) []byte {
	c := &ProductionChain{}
	h := c.calcBlockHash(b)
	return []byte(h)
}

func SignBlock(b *Block, priv ed25519.PrivateKey) {
	msg := blockSignHash(*b)
	sig := ed25519.Sign(priv, msg)
	b.Signature = hex.EncodeToString(sig)
}

func ValidateBlockSignature(b Block) error {
	if b.ValidatorID == "" {
		return fmt.Errorf("missing validator_id")
	}
	if b.Producer == "" {
		return fmt.Errorf("missing producer")
	}
	if b.ValidatorID != b.Producer {
		return fmt.Errorf("validator_id/producer mismatch")
	}
	pub, err := hex.DecodeString(b.ValidatorID)
	if err != nil {
		return fmt.Errorf("bad validator_id hex")
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("bad validator_id size")
	}
	if b.ProducerPubKey != "" && b.ProducerPubKey != b.ValidatorID {
		return fmt.Errorf("producer pubkey/validator_id mismatch")
	}
	sig, err := hex.DecodeString(b.Signature)
	if err != nil {
		return fmt.Errorf("bad signature hex")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("bad signature size")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), blockSignHash(b), sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// VerifyBlockSignature verifies signature against b.Producer (validator pubkey hex)
func VerifyBlockSignature(b Block) bool {
	return ValidateBlockSignature(b) == nil
}
