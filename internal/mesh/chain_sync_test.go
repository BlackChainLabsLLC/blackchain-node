package mesh

import (
	"testing"
	"time"
)

func TestReachablePeersForSyncSkipsSuppressed(t *testing.T) {
	now := time.Now()
	m := &meshDaemon{peers: map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072", Reachable: true},
		"127.0.0.1:7073": {Addr: "127.0.0.1:7073", Reachable: true, SuppressedUntil: now.Add(5 * time.Second)},
		"127.0.0.1:7074": {Addr: "127.0.0.1:7074", Reachable: false},
	}}

	got := m.reachablePeersForSync()
	if len(got) != 1 || got[0] != "127.0.0.1:7072" {
		t.Fatalf("reachablePeersForSync() = %v", got)
	}
}

func TestNotePeerHeightSampleRejectsInconsistentTip(t *testing.T) {
	m := &meshDaemon{peers: map[string]*Peer{
		"127.0.0.1:7072": {Addr: "127.0.0.1:7072", Reachable: true, LastHeight: 10, LastTip: "aaa"},
	}}

	ok := m.notePeerHeightSample("127.0.0.1:7072", 10, "bbb", 30*time.Second)
	if ok {
		t.Fatalf("expected inconsistent sample to be rejected")
	}
	p := m.peers["127.0.0.1:7072"]
	if p.SyncFailures == 0 || p.SuppressedUntil.IsZero() {
		t.Fatalf("expected suppression after inconsistent sample: %+v", *p)
	}
}

func TestBackoffForFailuresCaps(t *testing.T) {
	max := 30 * time.Second
	if got := backoffForFailures(1, max); got != 1*time.Second {
		t.Fatalf("failures=1 got %s", got)
	}
	if got := backoffForFailures(2, max); got != 2*time.Second {
		t.Fatalf("failures=2 got %s", got)
	}
	if got := backoffForFailures(10, max); got != max {
		t.Fatalf("failures=10 got %s want %s", got, max)
	}
}
