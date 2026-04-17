package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func mustWalletLikeAddr(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	addr, err := addrFromPubHex(hex.EncodeToString(pub))
	if err != nil {
		t.Fatalf("addrFromPubHex: %v", err)
	}
	return pub, priv, addr
}

func mustSignedTx(t *testing.T, fromPriv ed25519.PrivateKey, fromAddr, to string, amount, nonce, fee int64) Tx {
	t.Helper()
	pub := fromPriv.Public().(ed25519.PublicKey)
	tx, err := SignTx(fromAddr, to, amount, nonce, fee, hex.EncodeToString(pub), hex.EncodeToString(fromPriv))
	if err != nil {
		t.Fatalf("SignTx: %v", err)
	}
	return tx
}

func TestAddTxToMempoolRejectsDuplicateAndConflictingNonce(t *testing.T) {
	c := newProductionChain()

	_, fromPriv, fromAddr := mustWalletLikeAddr(t)
	_, _, toAddr := mustWalletLikeAddr(t)

	fromAcct := c.getOrCreateAccount(fromAddr)
	fromAcct.Balance = 100
	fromAcct.Nonce = 0

	tx1 := mustSignedTx(t, fromPriv, fromAddr, toAddr, 5, 0, BaseFee)
	if err := c.addTxToMempoolLocked(tx1); err != nil {
		t.Fatalf("addTxToMempoolLocked tx1: %v", err)
	}

	if err := c.addTxToMempoolLocked(tx1); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate rejection, got: %v", err)
	}

	txConflict := mustSignedTx(t, fromPriv, fromAddr, toAddr, 7, 0, BaseFee)
	if err := c.addTxToMempoolLocked(txConflict); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("expected conflicting nonce rejection, got: %v", err)
	}
}

func TestAddTxToMempoolRejectsWhenFull(t *testing.T) {
	c := newProductionChain()
	c.mempool = make([]Tx, MaxMempoolTxs)
	c.rebuildMempoolIndexLocked()

	_, fromPriv, fromAddr := mustWalletLikeAddr(t)
	_, _, toAddr := mustWalletLikeAddr(t)
	fromAcct := c.getOrCreateAccount(fromAddr)
	fromAcct.Balance = 100

	tx := mustSignedTx(t, fromPriv, fromAddr, toAddr, 1, 0, BaseFee)
	if err := c.addTxToMempoolLocked(tx); err == nil || !strings.Contains(err.Error(), "mempool full") {
		t.Fatalf("expected mempool full rejection, got: %v", err)
	}
}

func TestCompactMempoolDropsStaleAndMalformed(t *testing.T) {
	c := newProductionChain()

	_, fromPriv, fromAddr := mustWalletLikeAddr(t)
	_, _, toAddr := mustWalletLikeAddr(t)
	from := c.getOrCreateAccount(fromAddr)
	from.Balance = 100
	from.Nonce = 0

	validNowStale := mustSignedTx(t, fromPriv, fromAddr, toAddr, 5, 0, BaseFee)
	malformed := Tx{From: "", To: toAddr, Amount: 1, Nonce: 0, Fee: BaseFee}
	c.mempool = []Tx{validNowStale, malformed}

	// Simulate state progression that makes the first tx stale.
	from.Nonce = 1

	c.compactMempoolLocked()
	if got := len(c.mempool); got != 0 {
		t.Fatalf("expected compacted mempool to be empty, got %d", got)
	}
}

func TestApplyBlockRejectsBadTxNonce(t *testing.T) {
	c := newProductionChain()

	_, fromPriv, fromAddr := mustWalletLikeAddr(t)
	_, _, toAddr := mustWalletLikeAddr(t)
	vPub, vPriv, _ := mustWalletLikeAddr(t)
	validatorPubHex := hex.EncodeToString(vPub)

	from := c.getOrCreateAccount(fromAddr)
	from.Balance = 100
	from.Nonce = 0

	badNonceTx := mustSignedTx(t, fromPriv, fromAddr, toAddr, 5, 1, BaseFee)
	b := Block{
		Producer:     validatorPubHex,
		ValidatorID:  validatorPubHex,
		Height:       1,
		PrevHash:     "",
		TimeUTC:      time.Now().UTC(),
		Txs:          []Tx{badNonceTx},
		Reward:       blockRewardForHeight(1) + badNonceTx.Fee,
		ProducerAddr: toAddr,
	}
	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, vPriv)

	err := c.applyBlockLocked(b)
	if err == nil || !strings.Contains(err.Error(), "bad nonce in block") {
		t.Fatalf("expected bad nonce in block rejection, got: %v", err)
	}
}
