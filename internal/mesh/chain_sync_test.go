package mesh

import (
	"strings"
	"testing"
	"time"
)

func TestReachablePeersForSyncSkipsSuppressedPeers(t *testing.T) {
	now := time.Now()
	m := &meshDaemon{
		peers: map[string]*Peer{
			"127.0.0.1:7072": {Addr: "127.0.0.1:7072", Reachable: true},
			"127.0.0.1:7073": {Addr: "127.0.0.1:7073", Reachable: true, SuppressedUntil: now.Add(10 * time.Second)},
			"127.0.0.1:7074": {Addr: "127.0.0.1:7074", Reachable: false},
		},
	}

	got := m.reachablePeersForSync(now)
	if len(got) != 1 || got[0] != "127.0.0.1:7072" {
		t.Fatalf("unexpected sync peers: %v", got)
	}
}

func TestNotePeerHeightSampleRejectsConflictingTip(t *testing.T) {
	m := &meshDaemon{peers: map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072", LastHeight: 12, LastTip: "tip-a"},
	}}

	err := m.notePeerHeightSample("127.0.0.1:7072", 12, "tip-b")
	if err == nil || !strings.Contains(err.Error(), "conflicting tip") {
		t.Fatalf("expected conflicting tip rejection, got %v", err)
	}
}

func TestNotePeerHeightSampleRejectsStaleRegression(t *testing.T) {
	m := &meshDaemon{peers: map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072", LastHeight: 12, LastTip: "tip-a"},
	}}

	err := m.notePeerHeightSample("127.0.0.1:7072", 11, "tip-a")
	if err == nil || !strings.Contains(err.Error(), "stale height") {
		t.Fatalf("expected stale height rejection, got %v", err)
	}
}

func TestNoteSyncFailureSuppressesAndSuccessClears(t *testing.T) {
	m := &meshDaemon{peers: map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072", Reachable: true},
	}}

	suppressFor := m.noteSyncFailure("127.0.0.1:7072", errString("probe failed"))
	if suppressFor <= 0 {
		t.Fatalf("expected suppression duration, got %s", suppressFor)
	}
	if peers := m.reachablePeersForSync(time.Now()); len(peers) != 0 {
		t.Fatalf("expected peer to be suppressed from sync candidates, got %v", peers)
	}

	m.noteSyncSuccess("127.0.0.1:7072", 22, "tip-22")
	got := m.reachablePeersForSync(time.Now())
	if len(got) != 1 || got[0] != "127.0.0.1:7072" {
		t.Fatalf("expected peer to return after sync success, got %v", got)
	}
	if peer := m.peers["127.0.0.1:7072"]; peer.SyncFailures != 0 || !peer.SuppressedUntil.IsZero() {
		t.Fatalf("expected sync success to clear suppression, peer=%+v", peer)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
