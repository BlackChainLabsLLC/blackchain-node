package mesh

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
)

// Phase 1.3 — Signed Transactions
//
// Canonical sign bytes = SHA256(JSON({from,to,amount,nonce})).
//
// Address binding rule (matches current wallet scheme):
//   address = hex( SHA256(pubkey_bytes) )[0:40]    // first 20 bytes (40 hex chars)
//
// This prevents "I sign with my key but claim someone else's From".

type txCanon struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Amount int64  `json:"amount"`
	Nonce  int64  `json:"nonce"`
}

func txSignBytes(tx Tx) []byte {
	raw, _ := json.Marshal(txCanon{
		From:   tx.From,
		To:     tx.To,
		Amount: tx.Amount,
		Nonce:  tx.Nonce,
	})
	sum := sha256.Sum256(raw)
	return sum[:]
}

func addrFromPubHex(pubHex string) (string, error) {
	pubRaw, err := hex.DecodeString(pubHex)
	if err != nil {
		return "", fmt.Errorf("bad pubkey hex")
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("bad pubkey size")
	}
	sum := sha256.Sum256(pubRaw)
	// first 20 bytes => 40 hex chars
	return hex.EncodeToString(sum[:])[:40], nil
}

func VerifyTx(tx Tx) error {
	if tx.From == "" || tx.To == "" {
		return fmt.Errorf("missing from/to")
	}
	if tx.Amount <= 0 {
		return fmt.Errorf("bad amount")
	}
	if tx.Nonce < 0 {
		return fmt.Errorf("bad nonce")
	}
	if tx.PubKey == "" || tx.Signature == "" {
		return fmt.Errorf("missing pubkey/sig")
	}

	// address binding
	wantAddr, err := addrFromPubHex(tx.PubKey)
	if err != nil {
		return err
	}
	if tx.From != wantAddr {
		return fmt.Errorf("from/pubkey mismatch")
	}

	pubRaw, _ := hex.DecodeString(tx.PubKey)
	sigRaw, err := hex.DecodeString(tx.Signature)
	if err != nil {
		return fmt.Errorf("bad sig hex")
	}
	if len(sigRaw) != ed25519.SignatureSize {
		return fmt.Errorf("bad sig size")
	}

	if !ed25519.Verify(ed25519.PublicKey(pubRaw), txSignBytes(tx), sigRaw) {
		return fmt.Errorf("bad signature")
	}
	return nil
}

// SignTx creates PubKey+Signature on a tx using a wallet's raw keys (hex).
// The wallet address must match the pubkey-derived address (checked on verify).
func SignTx(fromAddr, to string, amount, nonce int64, pubHex, privHex string) (Tx, error) {
	if to == "" || amount <= 0 || nonce < 0 {
		return Tx{}, fmt.Errorf("bad tx inputs")
	}
	pubRaw, err := hex.DecodeString(pubHex)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return Tx{}, fmt.Errorf("bad pubkey")
	}
	privRaw, err := hex.DecodeString(privHex)
	if err != nil || len(privRaw) != ed25519.PrivateKeySize {
		return Tx{}, fmt.Errorf("bad privkey")
	}
	// ensure fromAddr matches pubkey scheme
	want, err := addrFromPubHex(pubHex)
	if err != nil {
		return Tx{}, err
	}
	if fromAddr != want {
		return Tx{}, fmt.Errorf("from/pubkey mismatch")
	}

	tx := Tx{
		From:   fromAddr,
		To:     to,
		Amount: amount,
		Nonce:  nonce,
		PubKey: pubHex,
	}
	sig := ed25519.Sign(ed25519.PrivateKey(privRaw), txSignBytes(tx))
	tx.Signature = hex.EncodeToString(sig)
	return tx, nil
}

// VerifyTxSignature is a compatibility wrapper used by chain_core gates.
// Returns true if the tx passes signature + binding validation.
func VerifyTxSignature(tx Tx) bool {
	return VerifyTx(tx) == nil
}

// fmtInt formats int64 deterministically in base-10 (used by merkle leaves).
func fmtInt(v int64) string {
	// strconv avoids fmt overhead and is deterministic
	return strconv.FormatInt(v, 10)
}
