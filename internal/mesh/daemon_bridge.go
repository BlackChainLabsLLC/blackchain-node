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
	"path/filepath"
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
	tlsCfg         *MeshTLS
	walletAddr     string
	bootstrapPeers []string
	listener       net.Listener
	peers          map[string]*Peer
	lock           sync.RWMutex
	httpSrv        *http.Server

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

	diag *runtimeDiagnostics
}

/* ===================== START DAEMON ===================== */

func StartMeshDaemon(ctx context.Context, opts *MeshDaemonOptions) (DaemonNode, error) {
	diag := newRuntimeDiagnostics()
	diag.setStartupPhase("load_config")

	cfg, err := LoadMeshConfig(opts.MeshConfigPath)
	if err != nil {
		diag.setHalted("config_load_failed")
		return nil, err
	}

	// ===== CONFIG SNAPSHOT (SOURCE OF TRUTH) =====
	log.Println("[mesh] ===== CONFIG SNAPSHOT =====")
	log.Println("[mesh] node_id =", cfg.NodeID)
	log.Println("[mesh] listen  =", cfg.Listen)
	log.Println("[mesh] http    =", cfg.HttpListen)
	log.Println("[mesh] peers   =", cfg.Peers)
	log.Println("[mesh] =================================")

	diag.setStartupPhase("open_mesh_listener")
	ln, err := meshListen(cfg.Listen, cfg.TLS)
	if err != nil {
		diag.setHalted("mesh_listener_failed")
		return nil, fmt.Errorf("listen %s: %w", cfg.Listen, err)
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

	walletPath := filepath.Join(dataDir, "wallet.json")

	diag.setStartupPhase("wallet_init")
	w, err := crypto.LoadOrCreateWallet(walletPath)
	if err != nil {
		diag.setHalted("wallet_load_failed")
		return nil, err
	}

	/* ===================== BOOTSTRAP PEERS ===================== */

	bootstrapPeers := LoadBootstrapPeers()

	// merge config peers + bootstrap peers
	peerSet := map[string]struct{}{}

	for _, p := range cfg.Peers {
		p = strings.TrimSpace(p)
		if p != "" {
			peerSet[p] = struct{}{}
		}
	}

	for _, p := range bootstrapPeers {
		p = strings.TrimSpace(p)
		if p != "" {
			peerSet[p] = struct{}{}
		}
	}

	finalPeers := make([]string, 0, len(peerSet))
	for p := range peerSet {
		finalPeers = append(finalPeers, p)
	}

	/* ===================== INITIAL PEER MAP ===================== */

	peers := make(map[string]*Peer, len(peerSet))
	for p := range peerSet {
		peers[p] = &Peer{Addr: p}
	}

	/* ===================== DAEMON INIT ===================== */

	m := &meshDaemon{
		tlsCfg:         cfg.TLS,
		chain:          newProductionChain(),
		dataDir:        dataDir,
		walletAddr:     w.Address,
		persistDir:     cfg.PersistDir,
		bootstrapPeers: finalPeers,

		listener:   ln,
		peers:      peers,
		id:         cfg.Listen,
		nodeID:     nodeName,
		inbox:      make([]Message, 0, 128),
		seen:       make(map[string]time.Time),
		reputation: make(map[string]*Reputation),
		uc:         make(map[string]*UCRecord),
		diag:       diag,
	}

	m.chain.ensureGenesisLocked()
	m.chain.daemon = m

	m.chain.dataDir = m.dataDir
	m.chain.persistDir = m.persistDir

	diag.setStartupPhase("snapshot_recovery")
	if _, err := m.chain.LoadSnapshotFromDisk(); err != nil {
		diag.setHalted("snapshot_recovery_failed")
		return nil, err
	}

	diag.setStartupPhase("replay_persisted_blocks")
	if err := m.chain.loadFromDisk(); err != nil {
		diag.setHalted("replay_failed")
		return nil, err
	}

	m.chain.daemon = m

	m.chain.mu.Lock()

	localID := m.chain.ValidatorIDLocked()
	if localID != "" && localID != "ERR_NO_VALIDATOR" {
		m.chain.observeValidatorLocked(localID)
	}

	m.chain.mu.Unlock()

	m.startLivenessLoop()
	m.startSignedStateLoop()
	m.startProposerLoop()

	/* ===================== PEER BOOTSTRAP ===================== */

	if len(m.bootstrapPeers) > 0 {

		log.Println("[mesh] bootstrap peers:", m.bootstrapPeers)

		go m.ConnectToPeers(m.bootstrapPeers)
		go m.bootstrapSync(ctx)

		go m.discoveryPromoteLoop(ctx)
		go m.discoveryEvictDeadLoop(ctx)
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
		diag.setStartupPhase("http_tls_prepare")

		httpHost := "127.0.0.1"
		if h, _, err := net.SplitHostPort(strings.TrimSpace(cfg.HttpListen)); err == nil && strings.TrimSpace(h) != "" {
			httpHost = strings.TrimSpace(h)
		} else if strings.TrimSpace(cfg.Host) != "" {
			httpHost = strings.TrimSpace(cfg.Host)
		}

		httpCertPath, httpKeyPath, err := ensureHTTPServerTLSFiles(dataDir, httpHost)
		if err != nil {
			diag.setHalted("http_tls_prepare_failed")
			return nil, fmt.Errorf("http tls files: %w", err)
		}

		mux := http.NewServeMux()

		m.registerChainHandlers(mux)

		m.httpSrv = &http.Server{
			Addr:    cfg.HttpListen,
			Handler: buildHTTPMiddleware(cfg)(mux),
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}

		mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {

			if r.Method == http.MethodPost {

				var req struct {
					Addr string `json:"addr"`
				}

				_ = json.NewDecoder(r.Body).Decode(&req)

				addr := strings.TrimSpace(req.Addr)

				if addr != "" {

					m.lock.Lock()

					if _, ok := m.peers[addr]; !ok {
						m.TouchReachable(addr, true)

						exists := false
						for _, bp := range m.bootstrapPeers {
							if bp == addr {
								exists = true
								break
							}
						}

						if !exists {
							m.bootstrapPeers = append(m.bootstrapPeers, addr)
						}
					}

					m.lock.Unlock()

					go m.ConnectToPeers([]string{addr})
				}
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
	diag.markStartupComplete()
	log.Printf("[state] ready state=%s phase=%s", diag.snapshot().State, diag.snapshot().StartupPhase)

	return m, nil
}

/* ===================== SHUTDOWN ===================== */

func (m *meshDaemon) Shutdown(ctx context.Context) error {

	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}

	if m.listener != nil {
		return m.listener.Close()
	}

	return nil
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
					m.diag.incPeerFailure(fmt.Sprintf("dial_failed:%s", addr))
					cancel()
					continue
				}

				_ = conn.Close()
				m.TouchReachable(addr, true)
				m.diag.clearPeerDegraded()
				cancel()
			}
		}
	}()
}
