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
	if stringsTrim(b.Producer) == "" {
		return fmt.Errorf("missing producer")
	}
	if stringsTrim(b.ValidatorID) == "" {
		return fmt.Errorf("missing validator_id")
	}
	if b.ValidatorID != b.Producer {
		return fmt.Errorf("validator identity mismatch")
	}
	if stringsTrim(b.ProducerPubKey) != "" && stringsTrim(b.ProducerPubKey) != b.Producer {
		return fmt.Errorf("producer_pubkey mismatch")
	}
	if stringsTrim(b.Signature) == "" {
		return fmt.Errorf("missing signature")
	}

	pub, err := hex.DecodeString(b.Producer)
	if err != nil {
		return fmt.Errorf("bad producer pub hex: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("bad producer pub length: got=%d want=%d", len(pub), ed25519.PublicKeySize)
	}

	sig, err := hex.DecodeString(b.Signature)
	if err != nil {
		return fmt.Errorf("bad signature hex: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("bad signature length: got=%d want=%d", len(sig), ed25519.SignatureSize)
	}

	if !ed25519.Verify(ed25519.PublicKey(pub), blockSignHash(b), sig) {
		return fmt.Errorf("invalid block signature")
	}
	return nil
}

// VerifyBlockSignature verifies signature against b.Producer (validator pubkey hex)
func VerifyBlockSignature(b Block) bool {
	return ValidateBlockSignature(b) == nil
}
