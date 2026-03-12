package mesh

import (
	"crypto/ed25519"
	"encoding/hex"
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

// VerifyBlockSignature verifies signature against b.Producer (validator pubkey hex)
func VerifyBlockSignature(b Block) bool {
	if b.Producer == "" || b.Signature == "" {
		return false
	}

	pub, err := hex.DecodeString(b.Producer)
	if err != nil {
		return false
	}

	sig, err := hex.DecodeString(b.Signature)
	if err != nil {
		return false
	}

	return ed25519.Verify(
		ed25519.PublicKey(pub),
		blockSignHash(b),
		sig,
	)
}
