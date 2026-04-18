package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"
)

type txSigner struct {
	addr    string
	pubHex  string
	privHex string
}

func newTxSigner(t *testing.T) txSigner {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate tx signer: %v", err)
	}
	pubHex := hex.EncodeToString(pub)
	addr, err := addrFromPubHex(pubHex)
	if err != nil {
		t.Fatalf("addr from pub: %v", err)
	}
	return txSigner{
		addr:    addr,
		pubHex:  pubHex,
		privHex: hex.EncodeToString(priv),
	}
}

func signedTestTx(t *testing.T, s txSigner, to string, amount, nonce, fee int64) Tx {
	t.Helper()
	tx, err := SignTx(s.addr, to, amount, nonce, fee, s.pubHex, s.privHex)
	if err != nil {
		t.Fatalf("sign tx: %v", err)
	}
	return tx
}

func TestAddTxToMempoolRejectsDuplicateAndConflictingNonce(t *testing.T) {
	c := newTestChain(t)
	sender := newTxSigner(t)
	recipient := newTxSigner(t)
	c.accounts[sender.addr] = &Account{Balance: 1000, Nonce: 0}

	tx1 := signedTestTx(t, sender, recipient.addr, 10, 0, BaseFee)
	if err := c.addTxToMempoolLocked(tx1); err != nil {
		t.Fatalf("add tx1: %v", err)
	}
	if err := c.addTxToMempoolLocked(tx1); err == nil {
		t.Fatalf("expected duplicate tx rejection")
	}

	txConflict := signedTestTx(t, sender, newTxSigner(t).addr, 11, 0, BaseFee)
	if err := c.addTxToMempoolLocked(txConflict); err == nil {
		t.Fatalf("expected conflicting sender nonce rejection")
	}
}

func TestAddTxToMempoolRejectsWhenFull(t *testing.T) {
	c := newTestChain(t)
	sender := newTxSigner(t)
	c.accounts[sender.addr] = &Account{Balance: 1_000_000, Nonce: 0}

	for i := 0; i < MaxMempoolTxs; i++ {
		tx := signedTestTx(t, sender, sender.addr[:39]+"0", 1, int64(i), BaseFee)
		c.accounts[sender.addr].Nonce = int64(i)
		if err := c.addTxToMempoolLocked(tx); err != nil {
			t.Fatalf("fill mempool idx=%d err=%v", i, err)
		}
	}
	c.accounts[sender.addr].Nonce = int64(MaxMempoolTxs)
	tx := signedTestTx(t, sender, sender.addr[:39]+"1", 1, int64(MaxMempoolTxs), BaseFee)
	if err := c.addTxToMempoolLocked(tx); err == nil {
		t.Fatalf("expected mempool full rejection")
	}
}

func TestCompactMempoolDropsStaleAndMalformed(t *testing.T) {
	c := newTestChain(t)
	sender := newTxSigner(t)
	recipient := newTxSigner(t)
	c.accounts[sender.addr] = &Account{Balance: 1000, Nonce: 0}

	tx1 := signedTestTx(t, sender, recipient.addr, 10, 0, BaseFee)
	if err := c.addTxToMempoolLocked(tx1); err != nil {
		t.Fatalf("add tx1: %v", err)
	}

	txBad := tx1
	txBad.Signature = "bad"
	c.mempool = append(c.mempool, txBad)
	txStale := signedTestTx(t, sender, recipient.addr, 5, 0, BaseFee)
	c.mempool = append(c.mempool, txStale)
	c.rebuildMempoolIndexLocked()

	c.accounts[sender.addr].Nonce = 1
	dropped := c.compactMempoolLocked()
	if dropped < 2 {
		t.Fatalf("expected at least 2 dropped txs, got %d", dropped)
	}
	if len(c.mempool) != 0 {
		t.Fatalf("expected mempool to be empty after stale compaction, got %d", len(c.mempool))
	}
}

func TestApplyBlockRejectsBadTxNonce(t *testing.T) {
	c := newTestChain(t)
	signer := testSigner{
		pubHex: c.ValidatorIDLocked(),
	}
	privHex, err := c.ValidatorPrivHexLocked()
	if err != nil {
		t.Fatalf("validator private key: %v", err)
	}
	privRaw, err := hex.DecodeString(privHex)
	if err != nil {
		t.Fatalf("decode validator private key: %v", err)
	}
	signer.priv = ed25519.PrivateKey(privRaw)

	sender := newTxSigner(t)
	recipient := newTxSigner(t)
	c.accounts[sender.addr] = &Account{Balance: 1000, Nonce: 0}

	tx1 := signedTestTx(t, sender, recipient.addr, 10, 0, BaseFee)
	tx2 := signedTestTx(t, sender, recipient.addr, 11, 2, BaseFee)
	b := makeSignedBlock(t, c, signer, 1, "", time.Now().UTC())
	b.Txs = []Tx{tx1, tx2}
	totalFees := tx1.Fee + tx2.Fee
	b.Reward = blockRewardForHeight(b.Height) + totalFees
	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, signer.priv)

	if err := c.applyBlockLocked(b); err == nil {
		t.Fatalf("expected bad nonce sequencing rejection")
	}
}
