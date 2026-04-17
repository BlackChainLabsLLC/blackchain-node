package mesh

import "testing"

func TestProposerReadyLockedUsesValidatorActionGate(t *testing.T) {
	c := newTestChain(t)
	m := &meshDaemon{
		nodeID: "node2",
		chain:  c,
	}

	c.mu.Lock()
	err := m.proposerReadyLocked()
	c.mu.Unlock()
	if err == nil {
		t.Fatalf("expected proposer readiness rejection for non-leader node")
	}

	m.nodeID = "node1"
	c.validatorPubHex = "bad"
	c.validatorPrivHex = "bad"
	c.mu.Lock()
	err = m.proposerReadyLocked()
	c.mu.Unlock()
	if err == nil {
		t.Fatalf("expected proposer readiness rejection for bad validator identity")
	}
}
