package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

type testSigner struct {
	pubHex string
	priv   ed25519.PrivateKey
}

func newTestChainWithSigner(t *testing.T) (*ProductionChain, testSigner) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	c := newProductionChain()
	c.validatorPubHex = hex.EncodeToString(pub)
	c.validatorPrivHex = hex.EncodeToString(priv)
	return c, testSigner{pubHex: hex.EncodeToString(pub), priv: priv}
}

func makeSignedBlock(t *testing.T, c *ProductionChain, s testSigner, height int64, prevHash string, ts time.Time) Block {
	t.Helper()
	b := Block{
		Producer:    s.pubHex,
		ValidatorID: s.pubHex,
		Height:      height,
		PrevHash:    prevHash,
		TimeUTC:     ts.UTC(),
		Reward:      blockRewardForHeight(height),
		Txs:         nil,
	}
	b.BalancesRoot = c.calcStateHashWithBlock(b)
	b.Hash = c.calcBlockHash(b)
	SignBlock(&b, s.priv)
	return b
}

func TestProposalHardening_DuplicateCommittedRejected(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	base := time.Now().UTC()
	b1 := makeSignedBlock(t, c, s, 1, "", base)

	applied, err := c.applyBlockOrBufferLocked(b1)
	if err != nil || !applied {
		t.Fatalf("first apply err=%v applied=%v", err, applied)
	}

	_, err = c.applyBlockOrBufferLocked(b1)
	if err == nil || !strings.Contains(err.Error(), "duplicate committed proposal") {
		t.Fatalf("expected duplicate committed rejection, got: %v", err)
	}
}

func TestProposalHardening_ConflictingPendingRejected(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	base := time.Now().UTC()
	b1 := makeSignedBlock(t, c, s, 1, "", base)
	b2a := makeSignedBlock(t, c, s, 2, b1.Hash, base.Add(1*time.Second))
	b2b := makeSignedBlock(t, c, s, 2, b1.Hash, base.Add(2*time.Second))

	applied, err := c.applyBlockOrBufferLocked(b2a)
	if err != nil || applied {
		t.Fatalf("first out-of-order should buffer; err=%v applied=%v", err, applied)
	}

	_, err = c.applyBlockOrBufferLocked(b2b)
	if err == nil || !strings.Contains(err.Error(), "conflicting pending proposal") {
		t.Fatalf("expected conflicting pending rejection, got: %v", err)
	}
}

func TestProposalHardening_OutOfOrderBufferedPath(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	base := time.Now().UTC()
	b1 := makeSignedBlock(t, c, s, 1, "", base)

	preview := newProductionChain()
	preview.validatorPubHex = c.validatorPubHex
	preview.validatorPrivHex = c.validatorPrivHex
	preview.ensureGenesisLocked()
	applied, err := preview.applyBlockOrBufferLocked(b1)
	if err != nil || !applied {
		t.Fatalf("preview apply b1 err=%v applied=%v", err, applied)
	}
	b2 := makeSignedBlock(t, preview, s, 2, b1.Hash, base.Add(1*time.Second))

	applied, err = c.applyBlockOrBufferLocked(b2)
	if err != nil || applied {
		t.Fatalf("buffer b2 err=%v applied=%v", err, applied)
	}

	applied, err = c.applyBlockOrBufferLocked(b1)
	if err != nil || !applied {
		t.Fatalf("apply b1 err=%v applied=%v", err, applied)
	}
	if c.height < 1 {
		t.Fatalf("expected forward progress to at least height=1, got=%d", c.height)
	}
	if c.height != 2 {
		t.Fatalf("expected buffered successor to drain after parent apply, got height=%d", c.height)
	}
}

func TestProposalHardening_FutureTimestampRejected(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	future := time.Now().UTC().Add(maxProposalFutureSkew + 30*time.Second)
	b1 := makeSignedBlock(t, c, s, 1, "", future)

	_, err := c.applyBlockOrBufferLocked(b1)
	if err == nil || !strings.Contains(err.Error(), "future timestamp") {
		t.Fatalf("expected future timestamp rejection, got: %v", err)
	}
}

func TestProposalHardening_StaleTimestampRejected(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	base := time.Now().UTC().Add(-10 * time.Second)
	b1 := makeSignedBlock(t, c, s, 1, "", base)

	applied, err := c.applyBlockOrBufferLocked(b1)
	if err != nil || !applied {
		t.Fatalf("apply b1 err=%v applied=%v", err, applied)
	}

	b2 := makeSignedBlock(t, c, s, 2, b1.Hash, base)
	_, err = c.applyBlockOrBufferLocked(b2)
	if err == nil || !strings.Contains(err.Error(), "stale timestamp") {
		t.Fatalf("expected stale timestamp rejection, got: %v", err)
	}
}

func TestProposalHardening_TooFarAheadRejected(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	base := time.Now().UTC()
	b3 := makeSignedBlock(t, c, s, 3, "unknown-prev", base.Add(3*time.Second))

	_, err := c.applyBlockOrBufferLocked(b3)
	if err == nil || !strings.Contains(err.Error(), "out-of-order proposal") {
		t.Fatalf("expected out-of-order rejection, got: %v", err)
	}
}

func TestProposalHardening_DropsStaleBufferedSuccessorOnParentAdvance(t *testing.T) {
	c, s := newTestChainWithSigner(t)
	base := time.Now().UTC()

	b2 := makeSignedBlock(t, c, s, 2, "stale-parent", base.Add(2*time.Second))
	applied, err := c.applyBlockOrBufferLocked(b2)
	if err != nil || applied {
		t.Fatalf("buffer stale successor err=%v applied=%v", err, applied)
	}
	if _, ok := c.pending[2]; !ok {
		t.Fatalf("expected height 2 proposal to be buffered")
	}

	b1 := makeSignedBlock(t, c, s, 1, "", base.Add(1*time.Second))
	applied, err = c.applyBlockOrBufferLocked(b1)
	if err != nil || !applied {
		t.Fatalf("apply parent err=%v applied=%v", err, applied)
	}
	if _, ok := c.pending[2]; ok {
		t.Fatalf("expected stale buffered successor to be dropped after parent advance")
	}
}

func TestProposalHardening_ProposalSafetyRejectsOtherValidatorPending(t *testing.T) {
	c, local := newTestChainWithSigner(t)
	_, remotePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate remote key: %v", err)
	}
	remotePubHex := hex.EncodeToString(remotePriv.Public().(ed25519.PublicKey))

	c.pending[2] = Block{
		Height:      2,
		Hash:        "remote-h2",
		PrevHash:    "remote-h1",
		Producer:    remotePubHex,
		TimeUTC:     time.Now().UTC(),
		ValidatorID: remotePubHex,
	}

	err = c.proposalSafetyCheckLocked(local.pubHex)
	if err == nil || !strings.Contains(err.Error(), "another validator") {
		t.Fatalf("expected other-validator safety rejection, got: %v", err)
	}
}
