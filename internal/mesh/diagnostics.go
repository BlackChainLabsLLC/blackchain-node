package mesh

import (
	"log"
	"sync"
	"time"
)

type nodeLifecycleState string

const (
	lifecycleStartup  nodeLifecycleState = "startup"
	lifecycleHealthy  nodeLifecycleState = "healthy"
	lifecycleDegraded nodeLifecycleState = "degraded"
	lifecycleHalted   nodeLifecycleState = "halted"
)

type runtimeDiagnostics struct {
	mu sync.RWMutex

	state        nodeLifecycleState
	startupPhase string
	haltedReason string

	replayFailures       uint64
	replayTailSkips      uint64
	replayAppliedBlocks  uint64
	syncFailures         uint64
	peerFailures         uint64
	proposalFailures     uint64
	recoverySnapshotUsed bool

	degradedReasons map[string]string
}

func newRuntimeDiagnostics() *runtimeDiagnostics {
	return &runtimeDiagnostics{
		state:           lifecycleStartup,
		startupPhase:    "bootstrap",
		degradedReasons: make(map[string]string),
	}
}

func (d *runtimeDiagnostics) setStartupPhase(phase string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.startupPhase = phase
	d.mu.Unlock()
	log.Printf("[startup] phase=%s", phase)
}

func (d *runtimeDiagnostics) setHalted(reason string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.state = lifecycleHalted
	d.haltedReason = reason
	d.mu.Unlock()
	log.Printf("[state] halted reason=%s", reason)
}

func (d *runtimeDiagnostics) markStartupComplete() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.startupPhase = "serving"
	if len(d.degradedReasons) > 0 {
		d.state = lifecycleDegraded
	} else {
		d.state = lifecycleHealthy
	}
	d.mu.Unlock()
}

func (d *runtimeDiagnostics) addDegradedReason(code string, detail string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.degradedReasons[code] = detail
	if d.state != lifecycleHalted {
		d.state = lifecycleDegraded
	}
	d.mu.Unlock()
}

func (d *runtimeDiagnostics) clearDegradedReason(code string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	delete(d.degradedReasons, code)
	if d.state != lifecycleHalted {
		if len(d.degradedReasons) == 0 && d.startupPhase == "serving" {
			d.state = lifecycleHealthy
		} else if len(d.degradedReasons) > 0 {
			d.state = lifecycleDegraded
		}
	}
	d.mu.Unlock()
}

func (d *runtimeDiagnostics) incReplayFailure(detail string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.replayFailures++
	d.mu.Unlock()
	d.addDegradedReason("replay_failures", detail)
}

func (d *runtimeDiagnostics) incReplayTailSkip() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.replayTailSkips++
	d.mu.Unlock()
}

func (d *runtimeDiagnostics) incReplayApplied(n uint64) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.replayAppliedBlocks += n
	d.mu.Unlock()
}

func (d *runtimeDiagnostics) incSyncFailure(detail string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.syncFailures++
	d.mu.Unlock()
	d.addDegradedReason("sync_failures", detail)
}

func (d *runtimeDiagnostics) clearSyncDegraded() {
	if d == nil {
		return
	}
	d.clearDegradedReason("sync_failures")
	d.clearDegradedReason("sync_waiting_for_reachable_peers")
}

func (d *runtimeDiagnostics) incPeerFailure(detail string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.peerFailures++
	d.mu.Unlock()
	d.addDegradedReason("peer_failures", detail)
}

func (d *runtimeDiagnostics) clearPeerDegraded() {
	if d == nil {
		return
	}
	d.clearDegradedReason("peer_failures")
}

func (d *runtimeDiagnostics) incProposalFailure(detail string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.proposalFailures++
	d.mu.Unlock()
	d.addDegradedReason("proposal_failures", detail)
}

func (d *runtimeDiagnostics) clearProposalDegraded() {
	if d == nil {
		return
	}
	d.clearDegradedReason("proposal_failures")
}

type diagnosticsSnapshot struct {
	State          string            `json:"state"`
	StartupPhase   string            `json:"startup_phase"`
	HaltedReason   string            `json:"halted_reason,omitempty"`
	Ready          bool              `json:"ready"`
	Live           bool              `json:"live"`
	UpdatedAtUTC   time.Time         `json:"updated_at_utc"`
	DegradedReason map[string]string `json:"degraded_reasons,omitempty"`
	Counters       map[string]uint64 `json:"counters"`
	Recovery       map[string]any    `json:"recovery"`
}

func (d *runtimeDiagnostics) snapshot() diagnosticsSnapshot {
	if d == nil {
		return diagnosticsSnapshot{
			State:        string(lifecycleStartup),
			StartupPhase: "bootstrap",
			Ready:        false,
			Live:         true,
			UpdatedAtUTC: time.Now().UTC(),
			Counters:     map[string]uint64{},
			Recovery:     map[string]any{},
		}
	}
	d.mu.RLock()
	degraded := make(map[string]string, len(d.degradedReasons))
	for k, v := range d.degradedReasons {
		degraded[k] = v
	}
	ready := d.state != lifecycleStartup && d.state != lifecycleHalted
	live := d.state != lifecycleHalted
	s := diagnosticsSnapshot{
		State:          string(d.state),
		StartupPhase:   d.startupPhase,
		HaltedReason:   d.haltedReason,
		Ready:          ready,
		Live:           live,
		UpdatedAtUTC:   time.Now().UTC(),
		DegradedReason: degraded,
		Counters: map[string]uint64{
			"replay_failures":       d.replayFailures,
			"replay_tail_skips":     d.replayTailSkips,
			"replay_applied_blocks": d.replayAppliedBlocks,
			"sync_failures":         d.syncFailures,
			"peer_failures":         d.peerFailures,
			"proposal_failures":     d.proposalFailures,
		},
		Recovery: map[string]any{
			"snapshot_loaded": d.recoverySnapshotUsed,
		},
	}
	d.mu.RUnlock()
	return s
}

func (d *runtimeDiagnostics) markSnapshotLoaded(loaded bool) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.recoverySnapshotUsed = loaded
	d.mu.Unlock()
}
