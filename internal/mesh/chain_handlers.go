package mesh

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

// active chain pointer (daemon-owned)
var activeChainMu sync.RWMutex
var activeChainPtr *ProductionChain

func setActiveChain(c *ProductionChain) {
	activeChainMu.Lock()
	activeChainPtr = c
	activeChainMu.Unlock()
}

func getActiveChain() *ProductionChain {
	activeChainMu.RLock()
	c := activeChainPtr
	activeChainMu.RUnlock()
	return c
}

// registerChainHandlers mounts core chain HTTP endpoints.
func (m *meshDaemon) registerChainHandlers(mux *http.ServeMux) {
	// --------------------
	// DEBUG: WALLET + DAEMON BINDING
	// GET /debug/wallet
	// --------------------
	mux.HandleFunc("/debug/wallet", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		daemonBound := (m.chain != nil && m.chain.daemon != nil)
		daemonWallet := ""
		if daemonBound {
			daemonWallet = m.chain.daemon.walletAddr
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"mesh_wallet":   m.walletAddr,
			"daemon_bound":  daemonBound,
			"daemon_wallet": daemonWallet,
			"data_dir":      m.dataDir,
		})
	})

	mux.HandleFunc("/chain/height", func(w http.ResponseWriter, _ *http.Request) {
		m.chain.mu.RLock()
		h := m.chain.height
		m.chain.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h})
	})
	// --------------------
	// STATUS
	// --------------------
	mux.HandleFunc("/chain/status", func(w http.ResponseWriter, r *http.Request) {
		m.chain.mu.RLock()
		defer m.chain.mu.RUnlock()

		_ = json.NewEncoder(w).Encode(map[string]any{
			"height": m.chain.height,
			"tip":    m.chain.tip,
		})
	})

	// --------------------
	// BLOCK BY HEIGHT
	// --------------------
	mux.HandleFunc("/chain/block", func(w http.ResponseWriter, r *http.Request) {
		hstr := r.URL.Query().Get("h")
		if hstr == "" {
			http.Error(w, "missing height", 400)
			return
		}

		h, err := strconv.ParseInt(hstr, 10, 64)
		if err != nil {
			http.Error(w, "bad height", 400)
			return
		}

		m.chain.mu.RLock()
		b, ok := m.chain.blocks[h]
		m.chain.mu.RUnlock()

		if !ok {
			http.Error(w, "not found", 404)
			return
		}

		_ = json.NewEncoder(w).Encode(b)
	})

	// --------------------
	// ADD TX (MEMPOOL ENTRY)
	// --------------------
	mux.HandleFunc("/chain/tx", func(w http.ResponseWriter, r *http.Request) {
		var tx Tx
		if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		m.chain.mu.Lock()
		defer m.chain.mu.Unlock()

		if err := m.chain.validateTx(tx); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		m.chain.mempool = append(m.chain.mempool, tx)

		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// --------------------
	// BALANCES (READ ONLY)
	// --------------------
	mux.HandleFunc("/chain/balances", func(w http.ResponseWriter, r *http.Request) {
		m.chain.mu.RLock()
		defer m.chain.mu.RUnlock()

		out := make(map[string]Account)
		for k, v := range m.chain.accounts {
			out[k] = *v
		}

		_ = json.NewEncoder(w).Encode(out)
	})

	// --------------------
	// MEMPOOL (debug visibility)
	// GET /chain/mempool
	// --------------------
	mux.HandleFunc("/chain/mempool", func(w http.ResponseWriter, r *http.Request) {
		m.chain.mu.RLock()
		defer m.chain.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"n":   len(m.chain.mempool),
			"txs": m.chain.mempool,
		})
	})

	// --------------------
	// APPLY BLOCK (SYNC)
	// --------------------
	mux.HandleFunc("/chain/apply", func(w http.ResponseWriter, r *http.Request) {
		var b Block
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		m.chain.mu.Lock()
		// HTTP_FINALITY_GUARD: finalized heights are immutable at the public apply endpoint
		fh := m.chain.finalizedHeight
		if fh > 0 && b.Height <= fh {
			m.chain.mu.Unlock()
			http.Error(w, fmt.Sprintf("finalized guard: height=%d finalized=%d", b.Height, fh), http.StatusConflict)
			return
		}

		_, err := m.chain.applyBlockOrBufferLocked(b)
		m.chain.mu.Unlock()

		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	// --------------------
	// LOCAL PROPOSE
	// --------------------
	mux.HandleFunc("/chain/propose", func(w http.ResponseWriter, r *http.Request) {
		// Leader gating: only the configured leader node may propose.
		if m.nodeID != "node1" {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "not leader", "node_id": m.nodeID})
			return
		}

		m.chain.mu.Lock()
		err := m.chain.proposeBlock()
		m.chain.mu.Unlock()

		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// --------------------
	// PROPOSE + GOSSIP
	// --------------------
	// --------------------
	// PROPOSE + GOSSIP
	// --------------------
	mux.HandleFunc("/chain/propose_broadcast", func(w http.ResponseWriter, r *http.Request) {
		// Leader gating: only the configured leader node may propose+broadcast.
		if m.nodeID != "node1" {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "not leader", "node_id": m.nodeID})
			return
		}

		m.chain.mu.Lock()
		err := m.chain.proposeBlock()
		if err != nil {
			m.chain.mu.Unlock()
			http.Error(w, err.Error(), 400)
			return
		}
		b := m.chain.blocks[m.chain.height]
		m.chain.mu.Unlock()
		if m != nil {
			go m.gossipBlock(b, 3)
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// DEBUG: identity inspection
	mux.HandleFunc("/debug/nodeid", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node_id": m.nodeID,
			"id":      m.id,
		})

	})

	// DEBUG: finality inspection (visibility only)
	// GET /debug/finality
	// GET /debug/finality?h=12   (includes block finalized flag for that height if present)
	mux.HandleFunc("/debug/finality", func(w http.ResponseWriter, r *http.Request) {
		m.chain.mu.RLock()
		defer m.chain.mu.RUnlock()

		resp := map[string]any{
			"height":           m.chain.height,
			"tip":              m.chain.tip,
			"finalized_height": m.chain.finalizedHeight,
			"validator_set_n":  len(m.chain.validatorSet),
		}

		hstr := r.URL.Query().Get("h")
		if hstr != "" {
			if hh, err := strconv.ParseInt(hstr, 10, 64); err == nil {
				if b, ok := m.chain.blocks[hh]; ok {
					resp["block_h"] = hh
					resp["block_hash"] = b.Hash
					resp["block_finalized"] = b.IsFinalized
				} else {
					resp["block_h"] = hh
					resp["block_missing"] = true
				}
			} else {
				resp["bad_h"] = hstr
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

}

// writeJSON is a tiny helper for handlers in this file.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
