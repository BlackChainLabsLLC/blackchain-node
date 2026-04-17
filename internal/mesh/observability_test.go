package mesh

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOperatorStatusSnapshotStateTransitions(t *testing.T) {
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	m := &meshDaemon{
		runCtx:       runCtx,
		runCancel:    runCancel,
		startedAt:    time.Unix(100, 0).UTC(),
		startupPhase: "replay_blocks",
		peers: map[string]*Peer{
			"127.0.0.1:7072": {Addr: "127.0.0.1:7072", Reachable: true},
		},
	}

	status := m.operatorStatusSnapshot()
	if got := status["daemon_state"]; got != "startup" {
		t.Fatalf("expected startup state, got %v", got)
	}
	if ready, _ := status["ready"].(bool); ready {
		t.Fatalf("startup state must not be ready")
	}
	if live, _ := status["live"].(bool); !live {
		t.Fatalf("startup state must remain live")
	}

	m.markStartupReady()
	status = m.operatorStatusSnapshot()
	if got := status["daemon_state"]; got != "healthy" {
		t.Fatalf("expected healthy state after startup ready, got %v", got)
	}

	m.recordSyncError(fmt.Errorf("sync probe failed"))
	status = m.operatorStatusSnapshot()
	if got := status["daemon_state"]; got != "degraded" {
		t.Fatalf("expected degraded state after sync error, got %v", got)
	}
	reasons, _ := status["degraded_reasons"].([]string)
	if len(reasons) == 0 || reasons[0] != "sync_errors_present" {
		t.Fatalf("expected sync degraded reason, got %v", status["degraded_reasons"])
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	status = m.operatorStatusSnapshot()
	if got := status["daemon_state"]; got != "halted" {
		t.Fatalf("expected halted state after shutdown, got %v", got)
	}
	if live, _ := status["live"].(bool); live {
		t.Fatalf("halted state must not be live")
	}
}

func TestOperatorStatusSnapshotCounters(t *testing.T) {
	m := &meshDaemon{
		startedAt:    time.Unix(100, 0).UTC(),
		startupPhase: "ready",
		startupReady: true,
		peers:        map[string]*Peer{},
	}

	m.recordRecoveryEvent("snapshot_tmp_promoted", "height=3")
	m.recordProposalFailure(fmt.Errorf("proposal rejected: bad hash"))
	m.recordPeerFailure("127.0.0.1:7072", "dial/connect failure")
	m.recordPeerSuppression("127.0.0.1:7072", time.Unix(200, 0).UTC(), "sync suppression")

	status := m.operatorStatusSnapshot()
	if got := status["recovery_event_count"]; got != int64(1) {
		t.Fatalf("expected one recovery event, got %v", got)
	}
	if got := status["proposal_failure_count"]; got != int64(1) {
		t.Fatalf("expected one proposal failure, got %v", got)
	}
	if got := status["peer_failure_count"]; got != int64(1) {
		t.Fatalf("expected one peer failure, got %v", got)
	}
	if got := status["peer_suppression_count"]; got != int64(1) {
		t.Fatalf("expected one peer suppression, got %v", got)
	}
	if msg, _ := status["last_recovery_event"].(string); !strings.Contains(msg, "snapshot_tmp_promoted") {
		t.Fatalf("expected recovery event detail, got %q", msg)
	}
	if msg, _ := status["last_proposal_failure"].(string); !strings.Contains(msg, "bad hash") {
		t.Fatalf("expected proposal failure detail, got %q", msg)
	}
}

func TestHealthzAndReadyzSemantics(t *testing.T) {
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	m := &meshDaemon{
		runCtx:       runCtx,
		runCancel:    runCancel,
		startedAt:    time.Unix(100, 0).UTC(),
		startupPhase: "replay_blocks",
		peers:        map[string]*Peer{},
		chain:        newProductionChain(),
	}
	m.chain.daemon = m

	mux := http.NewServeMux()
	m.registerChainHandlers(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected startup healthz 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected startup readyz 503, got %d", rr.Code)
	}

	m.markStartupReady()
	m.recordSyncError(fmt.Errorf("sync lagging"))

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected degraded healthz 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected degraded readyz 200, got %d", rr.Code)
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected halted healthz 503, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected halted readyz 503, got %d", rr.Code)
	}
}
