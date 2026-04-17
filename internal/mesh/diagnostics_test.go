package mesh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeDiagnosticsCountersAndStateTransitions(t *testing.T) {
	d := newRuntimeDiagnostics()
	if got := d.snapshot().State; got != string(lifecycleStartup) {
		t.Fatalf("initial state: got %s", got)
	}

	d.incReplayFailure("bad replay")
	d.incSyncFailure("sync err")
	d.incPeerFailure("peer err")
	d.incProposalFailure("proposal err")
	d.incReplayApplied(2)
	d.incReplayTailSkip()
	d.markSnapshotLoaded(true)
	d.markStartupComplete()

	s := d.snapshot()
	if s.State != string(lifecycleDegraded) {
		t.Fatalf("expected degraded state after failures, got %s", s.State)
	}
	if !s.Ready {
		t.Fatalf("degraded state should still be ready")
	}
	if s.Counters["replay_failures"] != 1 || s.Counters["sync_failures"] != 1 || s.Counters["peer_failures"] != 1 || s.Counters["proposal_failures"] != 1 {
		t.Fatalf("unexpected counters: %#v", s.Counters)
	}
	if s.Counters["replay_applied_blocks"] != 2 || s.Counters["replay_tail_skips"] != 1 {
		t.Fatalf("unexpected replay counters: %#v", s.Counters)
	}
	if loaded, ok := s.Recovery["snapshot_loaded"].(bool); !ok || !loaded {
		t.Fatalf("expected snapshot_loaded=true, got %#v", s.Recovery)
	}

	d.clearSyncDegraded()
	d.clearPeerDegraded()
	d.clearProposalDegraded()
	d.clearDegradedReason("replay_failures")
	s = d.snapshot()
	if s.State != string(lifecycleHealthy) {
		t.Fatalf("expected healthy after clear, got %s", s.State)
	}
}

func TestHealthEndpointsReadinessVsLiveness(t *testing.T) {
	d := newRuntimeDiagnostics()
	m := &meshDaemon{chain: newProductionChain(), diag: d}
	mux := http.NewServeMux()
	m.registerChainHandlers(mux)

	readyReq := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	readyRec := httptest.NewRecorder()
	mux.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("startup readiness code: got %d", readyRec.Code)
	}

	d.markStartupComplete()
	d.addDegradedReason("sync_failures", "sync still retrying")
	readyRec = httptest.NewRecorder()
	mux.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("degraded readiness code: got %d", readyRec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(readyRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readiness body: %v", err)
	}
	if body["ready"] != true {
		t.Fatalf("expected ready=true in degraded serving state")
	}

	liveReq := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	liveRec := httptest.NewRecorder()
	mux.ServeHTTP(liveRec, liveReq)
	if liveRec.Code != http.StatusOK {
		t.Fatalf("liveness code: got %d", liveRec.Code)
	}

	d.setHalted("fatal_startup_validation")
	liveRec = httptest.NewRecorder()
	mux.ServeHTTP(liveRec, liveReq)
	if liveRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("halted liveness code: got %d", liveRec.Code)
	}
}
