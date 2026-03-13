package mesh

import (
	"blackchain/internal/crypto"
	"context"
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
	Addr      string    `json:"addr"`
	LastSeen  time.Time `json:"last_seen"`
	Connected bool      `json:"connected"`
	Reachable bool      `json:"reachable"`
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
}

/* ===================== START DAEMON ===================== */

func StartMeshDaemon(ctx context.Context, opts *MeshDaemonOptions) (DaemonNode, error) {

	cfg, err := LoadMeshConfig(opts.MeshConfigPath)
	if err != nil {
		return nil, err
	}

	ln, err := meshListen(cfg.Listen, cfg.TLS)
	if err != nil {
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

	w, err := crypto.LoadOrCreateWallet(walletPath)
	if err != nil {
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

	/* ===================== DAEMON INIT ===================== */

	m := &meshDaemon{
		tlsCfg:         cfg.TLS,
		chain:          newProductionChain(),
		dataDir:        dataDir,
		walletAddr:     w.Address,
		persistDir:     cfg.PersistDir,
		bootstrapPeers: finalPeers,

		listener:   ln,
		peers:      make(map[string]*Peer),
		id:         cfg.Listen,
		nodeID:     nodeName,
		inbox:      make([]Message, 0, 128),
		seen:       make(map[string]time.Time),
		reputation: make(map[string]*Reputation),
		uc:         make(map[string]*UCRecord),
	}

	m.chain.ensureGenesisLocked()
	m.chain.daemon = m

	m.chain.dataDir = m.dataDir
	m.chain.persistDir = m.persistDir

	if _, err := m.chain.LoadSnapshotFromDisk(); err != nil {
		return nil, err
	}

	if err := m.chain.loadFromDisk(); err != nil {
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

	if cfg.HttpListen != "" {

		mux := http.NewServeMux()

		m.registerChainHandlers(mux)

		m.httpSrv = &http.Server{
			Addr:    cfg.HttpListen,
			Handler: buildHTTPMiddleware(cfg)(mux),
		}

		mux.HandleFunc("/peers", func(w http.ResponseWriter, _ *http.Request) {
			m.lock.RLock()
			defer m.lock.RUnlock()
			_ = json.NewEncoder(w).Encode(m.peers)
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

			if err := m.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Println("[mesh] http error:", err)
			}

		}()
	}

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
