package consensus

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type Vote struct {
	Height      uint64 `json:"height"`
	Round       uint32 `json:"round"`
	BlockHash   string `json:"block_hash"`   // hex
	ValidatorID string `json:"validator_id"` // hex(pubkey)
	Sig         string `json:"sig"`          // hex(signature)
}

func (v Vote) signingBytes() ([]byte, error) {
	// Canonical signing payload
	payload := struct {
		Height      uint64 `json:"height"`
		Round       uint32 `json:"round"`
		BlockHash   string `json:"block_hash"`
		ValidatorID string `json:"validator_id"`
	}{
		Height:      v.Height,
		Round:       v.Round,
		BlockHash:   v.BlockHash,
		ValidatorID: v.ValidatorID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

func SignVote(height uint64, round uint32, blockHashHex string, validatorID string, priv ed25519.PrivateKey) (Vote, error) {
	if blockHashHex == "" || validatorID == "" {
		return Vote{}, errors.New("missing block hash or validator id")
	}
	v := Vote{
		Height:      height,
		Round:       round,
		BlockHash:   blockHashHex,
		ValidatorID: validatorID,
	}
	msg, err := v.signingBytes()
	if err != nil {
		return Vote{}, err
	}
	sig := ed25519.Sign(priv, msg)
	v.Sig = hex.EncodeToString(sig)
	return v, nil
}

func VerifyVote(v Vote, pub ed25519.PublicKey) error {
	if v.Sig == "" || v.BlockHash == "" || v.ValidatorID == "" {
		return errors.New("vote missing fields")
	}
	if hex.EncodeToString(pub) != v.ValidatorID {
		return fmt.Errorf("validator id mismatch")
	}
	msg, err := v.signingBytes()
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(v.Sig)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, msg, sig) {
		return errors.New("bad signature")
	}
	return nil
}
