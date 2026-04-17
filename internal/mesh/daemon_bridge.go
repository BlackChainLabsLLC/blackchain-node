package mesh

import (
	"blackchain/internal/crypto"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

/* ===================== CONFIG / TYPES ===================== */

type MeshDaemonOptions struct {
	MeshConfigPath string
	DataDir        string
}

type DaemonNode interface {
	Shutdown(ctx context.Context) error
}

type Peer struct {
	Addr            string    `json:"addr"`
	LastSeen        time.Time `json:"last_seen"`
	Connected       bool      `json:"connected"`
	Reachable       bool      `json:"reachable"`
	FailureCount    int       `json:"failure_count,omitempty"`
	SyncFailures    int       `json:"sync_failures,omitempty"`
	SuppressedUntil time.Time `json:"suppressed_until,omitempty"`
	LastSyncErr     string    `json:"last_sync_err,omitempty"`
	LastTip         string    `json:"last_tip,omitempty"`
	LastHeight      int64     `json:"last_height,omitempty"`
	TrafficRecently bool      `json:"traffic_recently,omitempty"`
	LastSeenAgeSec  int64     `json:"last_seen_age_sec,omitempty"`
	ObservedState   string    `json:"observed_state,omitempty"`
}

type RouteEntry struct {
	PeerID     string    `json:"PeerID"`
	NextHop    string    `json:"NextHop"`
	Distance   int       `json:"Distance"`
	LastUpdate time.Time `json:"LastUpdate"`
}

type meshDaemon struct {
	tlsCfg                   *MeshTLS
	walletAddr               string
	bootstrapPeers           []string
	peerAPI                  map[string]string
	allowRuntimePeerMutation bool
	debugEndpointsEnabled    bool
	adminEndpointsEnabled    bool
	listener                 net.Listener
	peers                    map[string]*Peer
	lock                     sync.RWMutex
	httpSrv                  *http.Server
	runCtx                   context.Context
	runCancel                context.CancelFunc
	shutdownOnce             sync.Once
	shutdownErr              error

	id     string
	nodeID string
	inbox  []Message
	seen   map[string]time.Time

	repMu      sync.Mutex
	reputation map[string]*Reputation
	uc         map[string]*UCRecord

	chain *ProductionChain

	peerStateMu sync.Mutex
	peerState   map[string]SignedStateAnnouncement

	dataDir    string
	persistDir string

	discoMu    sync.RWMutex
	discoCfg   discoveryConfig
	discoPeers map[string]discoveredPeer

	statusMu                  sync.RWMutex
	startedAt                 time.Time
	startupPhase              string
	startupReady              bool
	shutdownInitiated         bool
	replayFailureCount        int64
	lastReplayError           string
	syncErrorCount            int64
	lastSyncError             string
	lastSyncLocalHeight       int64
	lastSyncBestHeight        int64
	peerFailureCount          int64
	lastPeerFailure           string
	peerSuppressionCount      int64
	lastPeerSuppression       string
	proposalFailureCount      int64
	lastProposalFailure       string
	recoveryEventCount        int64
	lastRecoveryEvent         string
	rejectedPeerMutationCount int64
	lastRejectedPeerMutation  string
}

/* ===================== START DAEMON ===================== */

func StartMeshDaemon(ctx context.Context, opts *MeshDaemonOptions) (DaemonNode, error) {

	cfg, err := LoadMeshConfig(opts.MeshConfigPath)
	if err != nil {
		return nil, err
	}

	if err := preflightBindCheck(cfg.Listen, cfg.HttpListen); err != nil {
		return nil, err
	}

	nodeName := strings.TrimSpace(cfg.NodeID)
	if nodeName == "" {
		nodeName = "node1"
	}

	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(cfg.DataDir)
	}
	if dataDir == "" {
		dataDir = filepath.Join("data", nodeName)
	}

	if err := validateStartupDependencies(cfg, dataDir); err != nil {
		return nil, err
	}

	bootstrapPeers, err := loadBootstrapPeersFromFile(defaultBootstrapConfigPath, candidateSelfAddresses(cfg))
	if err != nil {
		return nil, err
	}

	// ===== CONFIG SNAPSHOT (SOURCE OF TRUTH) =====
	log.Println("[mesh] ===== CONFIG SNAPSHOT =====")
	log.Println("[mesh] node_id =", cfg.NodeID)
	log.Println("[mesh] listen  =", cfg.Listen)
	log.Println("[mesh] http    =", cfg.HttpListen)
	log.Println("[mesh] peers   =", cfg.Peers)
	log.Println("[mesh] debug_http =", resolveHTTPSurfaceEnabled(cfg.DebugEndpointsEnabled, cfg.HttpListen))
	log.Println("[mesh] admin_http =", resolveHTTPSurfaceEnabled(cfg.AdminEndpointsEnabled, cfg.HttpListen))
	log.Println("[mesh] =================================")

	ln, err := meshListen(cfg.Listen, cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", cfg.Listen, err)
	}

	walletPath := filepath.Join(dataDir, "wallet.json")

	w, err := crypto.LoadOrCreateWallet(walletPath)
	if err != nil {
		return nil, err
	}

	/* ===================== BOOTSTRAP PEERS ===================== */

	// merge config peers + bootstrap peers
	peerSet, err := buildStartupPeerSet(cfg.Peers, bootstrapPeers)
	if err != nil {
		return nil, err
	}

	finalPeers := make([]string, 0, len(peerSet))
	for p := range peerSet {
		finalPeers = append(finalPeers, p)
	}
	sort.Strings(finalPeers)

	/* ===================== INITIAL PEER MAP ===================== */

	peers := make(map[string]*Peer, len(peerSet))
	for p := range peerSet {
		peers[p] = &Peer{Addr: p}
	}

	/* ===================== DAEMON INIT ===================== */

	m := &meshDaemon{
		tlsCfg:                   cfg.TLS,
		chain:                    newProductionChain(),
		dataDir:                  dataDir,
		walletAddr:               w.Address,
		persistDir:               cfg.PersistDir,
		bootstrapPeers:           finalPeers,
		peerAPI:                  normalizePeerAPIMap(cfg.PeerAPI),
		allowRuntimePeerMutation: cfg.AllowRuntimePeerMutation,
		debugEndpointsEnabled:    resolveHTTPSurfaceEnabled(cfg.DebugEndpointsEnabled, cfg.HttpListen),
		adminEndpointsEnabled:    resolveHTTPSurfaceEnabled(cfg.AdminEndpointsEnabled, cfg.HttpListen),

		listener:   ln,
		peers:      peers,
		id:         cfg.Listen,
		nodeID:     nodeName,
		inbox:      make([]Message, 0, 128),
		seen:       make(map[string]time.Time),
		reputation: make(map[string]*Reputation),
		uc:         make(map[string]*UCRecord),
		startedAt:  time.Now().UTC(),
	}
	m.runCtx, m.runCancel = context.WithCancel(ctx)
	m.setStartupPhase("config_loaded")
	m.setStartupPhase("replay_snapshot")

	m.chain.ensureGenesisLocked()
	m.chain.daemon = m

	m.chain.dataDir = m.dataDir
	m.chain.persistDir = m.persistDir

	snapshotLoaded, err := m.chain.LoadSnapshotFromDisk()
	if err != nil {
		m.recordReplayFailure(err)
		log.Printf("[startup] snapshot replay failed: %v", err)
		return nil, err
	}
	log.Printf("[startup] snapshot replay loaded=%v", snapshotLoaded)

	m.setStartupPhase("replay_blocks")
	if err := m.chain.loadFromDisk(); err != nil {
		m.recordReplayFailure(err)
		log.Printf("[startup] block replay failed: %v", err)
		return nil, err
	}

	m.chain.daemon = m

	m.setStartupPhase("validator_identity")
	m.chain.mu.Lock()

	localID, err := m.chain.EnsureValidatorIdentityLocked()
	if err != nil {
		m.chain.mu.Unlock()
		return nil, fmt.Errorf("startup validation: validator identity: %w", err)
	}
	if localID != "" && localID != "ERR_NO_VALIDATOR" {
		m.chain.observeValidatorLocked(localID)
	}

	m.chain.mu.Unlock()
	m.setStartupPhase("runtime_starting")
	m.markStartupReady()
	m.chain.mu.RLock()
	log.Printf("[startup] ready node_id=%s height=%d tip=%s peers=%d data_dir=%s", m.nodeID, m.chain.height, m.chain.tip, len(m.peers), m.dataDir)
	m.chain.mu.RUnlock()

	m.startLivenessLoop()
	m.startSignedStateLoop()
	m.startProposerLoop()

	/* ===================== PEER BOOTSTRAP ===================== */

	if len(m.bootstrapPeers) > 0 {

		log.Println("[mesh] bootstrap peers:", m.bootstrapPeers)

		go m.ConnectToPeers(m.bootstrapPeers)
		go m.bootstrapSync(m.runCtx)

		go m.discoveryPromoteLoop(m.runCtx)
		go m.discoveryEvictDeadLoop(m.runCtx)
	}

	log.Println("[mesh] listening on", cfg.Listen)

	go func() {
		for {
			conn, err := ln.Accept()

			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Println("[mesh] accept error:", err)
					return
				}
			}

			go m.handleIncoming(conn)
		}
	}()

	/* ===================== HTTP DEBUG API ===================== */

	// ===== FORCE HTTP LISTEN INVARIANT =====
	if strings.TrimSpace(cfg.HttpListen) == "" {
		cfg.HttpListen = ":6060"
		log.Println("[mesh] http_listen was empty → defaulting to", cfg.HttpListen)
	}

	if cfg.HttpListen != "" {
		httpCertPath, httpKeyPath, err := ensureHTTPServerTLSFiles(dataDir, httpHostForListen(cfg.HttpListen, cfg.Host), cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("http tls files: %w", err)
		}

		mux := http.NewServeMux()

		m.registerChainHandlers(mux)

		m.httpSrv = &http.Server{
			Addr:              cfg.HttpListen,
			Handler:           buildHTTPMiddleware(cfg)(mux),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       30 * time.Second,
			MaxHeaderBytes:    8 << 10,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}

		mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				if !m.adminEndpointsEnabled {
					log.Printf("[peers] rejected runtime mutation from=%s reason=admin_surface_disabled", r.RemoteAddr)
					m.recordRejectedPeerMutation("admin_surface_disabled")
					writeJSON(w, http.StatusForbidden, map[string]any{
						"ok":    false,
						"error": "admin endpoints disabled on this node",
					})
					return
				}
				if !m.allowRuntimePeerMutation {
					m.recordRejectedPeerMutation("disabled")
					log.Printf("[peers] rejected runtime mutation from=%s reason=disabled", r.RemoteAddr)
					writeJSON(w, http.StatusForbidden, map[string]any{
						"ok":    false,
						"error": "runtime peer mutation disabled; set allow_runtime_peer_mutation=true for explicit opt-in",
					})
					return
				}

				var req struct {
					Addr string `json:"addr"`
				}

				if err := decodeJSONBody(w, r, maxJSONBodyBytes, &req); err != nil {
					m.recordRejectedPeerMutation("bad_json")
					log.Printf("[peers] rejected runtime mutation from=%s reason=bad_json err=%v", r.RemoteAddr, err)
					writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error())
					return
				}

				addr := strings.TrimSpace(req.Addr)
				if addr == "" {
					m.recordRejectedPeerMutation("empty_addr")
					log.Printf("[peers] rejected runtime mutation from=%s reason=empty_addr", r.RemoteAddr)
					writeAPIError(r, w, http.StatusBadRequest, "missing_addr", "missing addr")
					return
				}
				normalized, err := validateTCPAddress("peer", addr)
				if err != nil {
					m.recordRejectedPeerMutation("invalid_addr")
					log.Printf("[peers] rejected runtime mutation from=%s reason=invalid_addr addr=%q err=%v", r.RemoteAddr, addr, err)
					writeAPIError(r, w, http.StatusBadRequest, "invalid_addr", err.Error())
					return
				}
				if normalized == m.id {
					m.recordRejectedPeerMutation("self")
					log.Printf("[peers] rejected runtime mutation from=%s reason=self addr=%s", r.RemoteAddr, normalized)
					writeAPIError(r, w, http.StatusBadRequest, "self_peer", "peer mutation rejected: self address not allowed")
					return
				}

				m.lock.Lock()
				_, existsPeer := m.peers[normalized]
				existsBootstrap := false
				for _, bp := range m.bootstrapPeers {
					if bp == normalized {
						existsBootstrap = true
						break
					}
				}
				if !existsPeer {
					m.TouchReachable(normalized, true)
				}
				if !existsBootstrap {
					m.bootstrapPeers = append(m.bootstrapPeers, normalized)
				}
				m.lock.Unlock()

				if existsPeer && existsBootstrap {
					log.Printf("[peers] runtime mutation no-op from=%s addr=%s reason=already_present", r.RemoteAddr, normalized)
					writeJSON(w, http.StatusOK, map[string]any{
						"ok":     true,
						"status": "unchanged",
						"addr":   normalized,
					})
					return
				}

				log.Printf("[peers] accepted runtime mutation from=%s addr=%s", r.RemoteAddr, normalized)
				go m.ConnectToPeers([]string{normalized})
				writeJSON(w, http.StatusOK, map[string]any{
					"ok":     true,
					"status": "added",
					"addr":   normalized,
				})
				return
			}
			if !requireMethod(w, r, http.MethodGet) {
				return
			}

			m.lock.RLock()
			out := make(map[string]Peer, len(m.peers))
			now := time.Now()
			for addr, p := range m.peers {
				pp := *p
				if !pp.LastSeen.IsZero() {
					age := int64(now.Sub(pp.LastSeen) / time.Second)
					if age < 0 {
						age = 0
					}
					pp.LastSeenAgeSec = age
					pp.TrafficRecently = now.Sub(pp.LastSeen) <= LivenessWindow
				}
				switch {
				case pp.Connected && pp.Reachable && pp.TrafficRecently:
					pp.ObservedState = "healthy"
				case pp.Reachable && pp.TrafficRecently:
					pp.ObservedState = "reachable_recent_traffic"
				case pp.Reachable:
					pp.ObservedState = "reachable_no_recent_traffic"
				case pp.Connected:
					pp.ObservedState = "connected_state_only"
				default:
					pp.ObservedState = "stale_or_unreachable"
				}
				out[addr] = pp
			}
			m.lock.RUnlock()

			_ = json.NewEncoder(w).Encode(out)
		})

		mux.HandleFunc("/routes", func(w http.ResponseWriter, _ *http.Request) {

			m.lock.RLock()
			defer m.lock.RUnlock()

			routes := make([]RouteEntry, 0, len(m.peers))

			for addr, p := range m.peers {

				last := p.LastSeen

				if last.IsZero() {
					last = time.Now()
				}

				routes = append(routes, RouteEntry{
					PeerID:     addr,
					NextHop:    addr,
					Distance:   1,
					LastUpdate: last,
				})
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"routes": routes,
			})
		})

		go func() {

			log.Println("[mesh] http API →", cfg.HttpListen)

			if err := m.httpSrv.ListenAndServeTLS(httpCertPath, httpKeyPath); err != nil && err != http.ErrServerClosed {
				log.Println("[mesh] http error:", err)
			}

		}()
	}

	return m, nil
}

func buildStartupPeerSet(configPeers, bootstrapPeers []string) (map[string]struct{}, error) {
	peerSet := make(map[string]struct{}, len(configPeers)+len(bootstrapPeers))
	for _, src := range [][]string{configPeers, bootstrapPeers} {
		for _, p := range src {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			addr, err := validateTCPAddress("startup peer", p)
			if err != nil {
				return nil, err
			}
			peerSet[addr] = struct{}{}
		}
	}
	return peerSet, nil
}

/* ===================== SHUTDOWN ===================== */

func (m *meshDaemon) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do(func() {
		m.statusMu.Lock()
		m.shutdownInitiated = true
		m.statusMu.Unlock()
		log.Printf("[startup] phase=shutdown")
		if m.runCancel != nil {
			m.runCancel()
		}
		if m.httpSrv != nil {
			if err := m.httpSrv.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
				m.shutdownErr = err
				return
			}
		}
		if m.listener != nil {
			if err := m.listener.Close(); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
				m.shutdownErr = err
				return
			}
		}
	})
	return m.shutdownErr
}

func (m *meshDaemon) markStartupReady() {
	m.statusMu.Lock()
	m.startupPhase = "ready"
	m.startupReady = true
	m.statusMu.Unlock()
}

func (m *meshDaemon) setStartupPhase(phase string) {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return
	}
	m.statusMu.Lock()
	if m.startupPhase == phase {
		m.statusMu.Unlock()
		return
	}
	m.startupPhase = phase
	m.statusMu.Unlock()
	log.Printf("[startup] phase=%s", phase)
}

func (m *meshDaemon) recordReplayFailure(err error) {
	if err == nil {
		return
	}
	m.statusMu.Lock()
	m.replayFailureCount++
	m.lastReplayError = err.Error()
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordSyncHeights(localH, bestH int64) {
	m.statusMu.Lock()
	m.lastSyncLocalHeight = localH
	m.lastSyncBestHeight = bestH
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordSyncError(err error) {
	if err == nil {
		return
	}
	m.statusMu.Lock()
	m.syncErrorCount++
	m.lastSyncError = err.Error()
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordPeerFailure(addr, reason string) {
	if strings.TrimSpace(reason) == "" {
		return
	}
	msg := strings.TrimSpace(reason)
	if strings.TrimSpace(addr) != "" {
		msg = fmt.Sprintf("peer=%s %s", strings.TrimSpace(addr), msg)
	}
	m.statusMu.Lock()
	m.peerFailureCount++
	m.lastPeerFailure = msg
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordPeerSuppression(addr string, until time.Time, reason string) {
	msg := strings.TrimSpace(reason)
	if strings.TrimSpace(addr) != "" {
		msg = fmt.Sprintf("peer=%s until=%s %s", strings.TrimSpace(addr), until.UTC().Format(time.RFC3339), msg)
	}
	m.statusMu.Lock()
	m.peerSuppressionCount++
	m.lastPeerSuppression = strings.TrimSpace(msg)
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordProposalFailure(err error) {
	if err == nil {
		return
	}
	m.statusMu.Lock()
	m.proposalFailureCount++
	m.lastProposalFailure = err.Error()
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordRecoveryEvent(kind, detail string) {
	msg := strings.TrimSpace(kind)
	if strings.TrimSpace(detail) != "" {
		msg = fmt.Sprintf("%s: %s", msg, strings.TrimSpace(detail))
	}
	m.statusMu.Lock()
	m.recoveryEventCount++
	m.lastRecoveryEvent = msg
	m.statusMu.Unlock()
}

func (m *meshDaemon) recordRejectedPeerMutation(reason string) {
	m.statusMu.Lock()
	m.rejectedPeerMutationCount++
	m.lastRejectedPeerMutation = reason
	m.statusMu.Unlock()
}

func (m *meshDaemon) runtimeStateSnapshot() (state string, reasons []string, live bool, ready bool) {
	m.statusMu.RLock()
	startupPhase := m.startupPhase
	startupReady := m.startupReady
	shutdownInitiated := m.shutdownInitiated
	replayFailureCount := m.replayFailureCount
	syncErrorCount := m.syncErrorCount
	peerFailureCount := m.peerFailureCount
	peerSuppressionCount := m.peerSuppressionCount
	proposalFailureCount := m.proposalFailureCount
	m.statusMu.RUnlock()

	if shutdownInitiated {
		return "halted", []string{"shutdown_initiated"}, false, false
	}
	if !startupReady {
		reasons = append(reasons, fmt.Sprintf("startup_phase=%s", startupPhase))
		return "startup", reasons, true, false
	}
	if replayFailureCount > 0 {
		reasons = append(reasons, "replay_failures_present")
	}
	if syncErrorCount > 0 {
		reasons = append(reasons, "sync_errors_present")
	}
	if peerFailureCount > 0 {
		reasons = append(reasons, "peer_failures_present")
	}
	if peerSuppressionCount > 0 {
		reasons = append(reasons, "peer_suppressions_present")
	}
	if proposalFailureCount > 0 {
		reasons = append(reasons, "proposal_failures_present")
	}
	if len(reasons) > 0 {
		return "degraded", reasons, true, true
	}
	return "healthy", nil, true, true
}

func (m *meshDaemon) operatorStatusSnapshot() map[string]any {
	m.statusMu.RLock()
	startedAt := m.startedAt
	startupPhase := m.startupPhase
	startupReady := m.startupReady
	shutdownInitiated := m.shutdownInitiated
	replayFailureCount := m.replayFailureCount
	lastReplayError := m.lastReplayError
	syncErrorCount := m.syncErrorCount
	lastSyncError := m.lastSyncError
	lastSyncLocalHeight := m.lastSyncLocalHeight
	lastSyncBestHeight := m.lastSyncBestHeight
	peerFailureCount := m.peerFailureCount
	lastPeerFailure := m.lastPeerFailure
	peerSuppressionCount := m.peerSuppressionCount
	lastPeerSuppression := m.lastPeerSuppression
	proposalFailureCount := m.proposalFailureCount
	lastProposalFailure := m.lastProposalFailure
	recoveryEventCount := m.recoveryEventCount
	lastRecoveryEvent := m.lastRecoveryEvent
	rejectedPeerMutationCount := m.rejectedPeerMutationCount
	lastRejectedPeerMutation := m.lastRejectedPeerMutation
	m.statusMu.RUnlock()

	reachable := 0
	total := 0
	m.lock.RLock()
	total = len(m.peers)
	for _, p := range m.peers {
		if p.Reachable {
			reachable++
		}
	}
	m.lock.RUnlock()

	syncLag := int64(0)
	if lastSyncBestHeight > lastSyncLocalHeight {
		syncLag = lastSyncBestHeight - lastSyncLocalHeight
	}
	runtimeState, degradedReasons, live, ready := m.runtimeStateSnapshot()

	return map[string]any{
		"daemon_state":                        runtimeState,
		"degraded_reasons":                    degradedReasons,
		"live":                                live,
		"ready":                               ready,
		"started_at":                          startedAt.Format(time.RFC3339),
		"startup_phase":                       startupPhase,
		"startup_ready":                       startupReady,
		"shutdown_initiated":                  shutdownInitiated,
		"replay_failure_count":                replayFailureCount,
		"last_replay_error":                   lastReplayError,
		"sync_error_count":                    syncErrorCount,
		"last_sync_error":                     lastSyncError,
		"last_sync_local_height":              lastSyncLocalHeight,
		"last_sync_best_height":               lastSyncBestHeight,
		"sync_lag":                            syncLag,
		"peer_failure_count":                  peerFailureCount,
		"last_peer_failure":                   lastPeerFailure,
		"peer_suppression_count":              peerSuppressionCount,
		"last_peer_suppression":               lastPeerSuppression,
		"proposal_failure_count":              proposalFailureCount,
		"last_proposal_failure":               lastProposalFailure,
		"recovery_event_count":                recoveryEventCount,
		"last_recovery_event":                 lastRecoveryEvent,
		"reachable_peer_count":                reachable,
		"configured_peer_count":               total,
		"rejected_runtime_peer_mutations":     rejectedPeerMutationCount,
		"last_rejected_runtime_peer_mutation": lastRejectedPeerMutation,
	}
}

func preflightBindCheck(addrs ...string) error {
	held := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			for _, heldL := range held {
				_ = heldL.Close()
			}
			return fmt.Errorf("startup validation: address %s is unavailable: %w", addr, err)
		}
		held = append(held, l)
	}
	for _, l := range held {
		_ = l.Close()
	}
	return nil
}

func validateStartupDependencies(cfg *MeshConfig, dataDir string) error {
	if cfg == nil {
		return fmt.Errorf("startup validation: nil mesh config")
	}
	if err := validateRuntimePaths(dataDir, cfg.PersistDir); err != nil {
		return err
	}
	if cfg.TLS != nil {
		if err := cfg.TLS.validate(); err != nil {
			return err
		}
	}
	if _, _, err := ensureHTTPServerTLSFiles(dataDir, httpHostForListen(cfg.HttpListen, cfg.Host), cfg.TLS); err != nil {
		return fmt.Errorf("startup validation: http tls material: %w", err)
	}
	return nil
}

func validateRuntimePaths(dataDir, persistDir string) error {
	if err := ensureDirAccessible("data_dir", dataDir); err != nil {
		return err
	}
	if strings.TrimSpace(persistDir) != "" {
		if err := ensureDirAccessible("persist_dir", persistDir); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirAccessible(label, dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("startup validation: %s must not be empty", label)
	}
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("startup validation: %s path must be a directory: %s", label, dir)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("startup validation: stat %s %s: %w", label, dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("startup validation: create %s %s: %w", label, dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("startup validation: stat %s %s: %w", label, dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("startup validation: %s path must be a directory: %s", label, dir)
	}
	f, err := os.CreateTemp(dir, ".startup-check-*")
	if err != nil {
		return fmt.Errorf("startup validation: %s is not writable: %s: %w", label, dir, err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func httpHostForListen(httpListen, fallbackHost string) string {
	httpHost := "127.0.0.1"
	if h, _, err := net.SplitHostPort(strings.TrimSpace(httpListen)); err == nil && strings.TrimSpace(h) != "" {
		httpHost = strings.TrimSpace(h)
	} else if strings.TrimSpace(fallbackHost) != "" {
		httpHost = strings.TrimSpace(fallbackHost)
	}
	return httpHost
}

func resolveHTTPSurfaceEnabled(override *bool, httpListen string) bool {
	if override != nil {
		return *override
	}
	return isLoopbackHTTPListen(httpListen)
}

func isLoopbackHTTPListen(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizePeerAPIMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for meshAddr, apiAddr := range in {
		meshNormalized, err := validateTCPAddress("peer_api mesh address", strings.TrimSpace(meshAddr))
		if err != nil {
			continue
		}
		apiNormalized, err := validateTCPAddress("peer_api api address", strings.TrimSpace(apiAddr))
		if err != nil {
			continue
		}
		out[meshNormalized] = apiNormalized
	}
	return out
}

// startPeerDialLoop periodically attempts connections to all known peers.
// Source of truth is m.peers, not only bootstrapPeers.
func (m *meshDaemon) startPeerDialLoop() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			m.lock.Lock()
			peers := make([]string, 0, len(m.peers))
			for addr := range m.peers {
				addr = strings.TrimSpace(addr)
				if addr != "" {
					peers = append(peers, addr)
				}
			}
			m.lock.Unlock()

			for _, addr := range peers {
				if addr == "" {
					continue
				}

				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				conn, err := meshDialTimeout(ctx, addr, 2*time.Second, m.tlsCfg)
				if err != nil {
					m.TouchReachable(addr, false)
					cancel()
					continue
				}

				_ = conn.Close()
				m.TouchReachable(addr, true)
				cancel()
			}
		}
	}()
}
