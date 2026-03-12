package mesh

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Vote is a validator's signature over (height, block_hash).
type Vote struct {
	Height      int64  `json:"height"`
	BlockHash   string `json:"block_hash"`
	ValidatorID string `json:"validator_id"` // pubkey hex
	Signature   string `json:"sig"`          // signature hex
}

// voteSignBytes returns canonical bytes to sign for a vote.
func voteSignBytes(height int64, blockHash string) []byte {
	type canon struct {
		Height    int64  `json:"height"`
		BlockHash string `json:"block_hash"`
	}
	raw, _ := json.Marshal(canon{Height: height, BlockHash: blockHash})
	sum := sha256.Sum256(raw)
	return sum[:]
}

// SignVoteLocked creates a signed vote using the node's validator private key.
// Caller must hold c.mu (write or read; we may lazily load keys).
func (c *ProductionChain) SignVoteLocked(height int64, blockHash string) (Vote, error) {
	vid := c.ValidatorIDLocked()
	privHex := c.validatorPrivHex
	if vid == "" || privHex == "" || vid == "ERR_NO_VALIDATOR" {
		return Vote{}, fmt.Errorf("validator identity not available")
	}
	privRaw, err := hex.DecodeString(privHex)
	if err != nil || len(privRaw) != ed25519.PrivateKeySize {
		return Vote{}, fmt.Errorf("bad validator private key")
	}
	sig := ed25519.Sign(ed25519.PrivateKey(privRaw), voteSignBytes(height, blockHash))
	return Vote{
		Height:      height,
		BlockHash:   blockHash,
		ValidatorID: vid,
		Signature:   hex.EncodeToString(sig),
	}, nil
}

// VerifyVote verifies the vote signature using ValidatorID as pubkey hex.
func VerifyVote(v Vote) bool {
	if v.Height <= 0 || v.BlockHash == "" || v.ValidatorID == "" || v.Signature == "" {
		return false
	}
	pubRaw, err := hex.DecodeString(v.ValidatorID)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return false
	}
	sigRaw, err := hex.DecodeString(v.Signature)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pubRaw), voteSignBytes(v.Height, v.BlockHash), sigRaw)
}
