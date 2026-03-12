package chain

import (
	"blackchain/internal/mesh"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// HashBlockForSign returns the message bytes that are signed for a block.
// We sign a stable digest of the block's deterministic Hash field.
// (The Hash field itself should already be computed deterministically by the chain.)
func HashBlockForSign(b mesh.Block) []byte {
	h := sha256.Sum256([]byte(b.Hash))
	return h[:]
}

func SignBlock(b *mesh.Block, priv ed25519.PrivateKey, pub ed25519.PublicKey) {
	msg := HashBlockForSign(*b)
	sig := ed25519.Sign(priv, msg)
	b.ProducerPubKey = hex.EncodeToString(pub)
	b.Signature = hex.EncodeToString(sig)
}

func VerifyBlockSignature(b mesh.Block) error {
	if b.ProducerPubKey == "" || b.Signature == "" {
		return errors.New("missing block signature")
	}

	pub, err := hex.DecodeString(b.ProducerPubKey)
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(b.Signature)
	if err != nil {
		return err
	}

	msg := HashBlockForSign(b)
	if !ed25519.Verify(ed25519.PublicKey(pub), msg, sig) {
		return errors.New("invalid block signature")
	}
	return nil
}
