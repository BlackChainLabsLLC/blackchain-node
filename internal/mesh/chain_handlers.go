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
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
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

	mux.HandleFunc("/chain/height", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
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
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		m.chain.mu.RLock()
		height := m.chain.height
		tip := m.chain.tip
		finalizedHeight := m.chain.finalizedHeight
		finalizedTip := ""
		if finalizedHeight > 0 {
			if b, ok := m.chain.blocks[finalizedHeight]; ok {
				finalizedTip = b.Hash
			}
		}
		m.chain.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"height":           height,
			"tip":              tip,
			"finalized_height": finalizedHeight,
			"finalized_tip":    finalizedTip,
			"finality_depth":   finalityDepth,
		})
	})

	// ===== PHASE 9: /chain/finality =====
	mux.HandleFunc("/chain/finality", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		h, tip, depth := m.chain.GetFinalitySnapshot()

		resp := map[string]any{
			"finalized_height": h,
			"finalized_tip":    tip,
			"depth":            depth,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// --------------------
	// BLOCK BY HEIGHT
	// --------------------
	mux.HandleFunc("/chain/block", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		hstr := r.URL.Query().Get("h")
		if hstr == "" {
			writeAPIError(w, r, http.StatusBadRequest, "missing_height", "query parameter h is required")
			return
		}

		h, err := strconv.ParseInt(hstr, 10, 64)
		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "bad_height", "query parameter h must be an integer")
			return
		}

		m.chain.mu.RLock()
		b, ok := m.chain.blocks[h]
		m.chain.mu.RUnlock()

		if !ok {
			writeAPIError(w, r, http.StatusNotFound, "block_not_found", "block was not found at requested height")
			return
		}

		_ = json.NewEncoder(w).Encode(b)
	})

	// --------------------
	// ADD TX (MEMPOOL ENTRY)
	// --------------------
	mux.HandleFunc("/chain/tx", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		var tx Tx
		if !decodeJSONBody(w, r, &tx, maxJSONBodySmall) {
			return
		}

		m.chain.mu.Lock()
		defer m.chain.mu.Unlock()

		if err := m.chain.validateTx(tx); err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "tx_validation_failed", err.Error())
			return
		}

		m.chain.mempool = append(m.chain.mempool, tx)

		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// --------------------
	// BALANCES (READ ONLY)
	// --------------------
	mux.HandleFunc("/chain/balances", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
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
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
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
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		var b Block
		if !decodeJSONBody(w, r, &b, maxJSONBodyLarge) {
			return
		}

		m.chain.mu.Lock()
		// HTTP_FINALITY_GUARD: finalized heights are immutable at the public apply endpoint
		fh := m.chain.finalizedHeight
		if fh > 0 && b.Height <= fh {
			m.chain.mu.Unlock()
			writeAPIError(w, r, http.StatusConflict, "finalized_guard_violation", fmt.Sprintf("finalized guard: height=%d finalized=%d", b.Height, fh))
			return
		}

		_, err := m.chain.applyBlockOrBufferLocked(b)
		m.chain.mu.Unlock()

		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "apply_failed", err.Error())
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	// --------------------
	// LOCAL PROPOSE
	// --------------------
	mux.HandleFunc("/chain/propose", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		// Leader gating: only the configured leader node may propose.
		if m.nodeID != "node1" {
			writeAPIError(w, r, http.StatusForbidden, "not_leader", "only node1 is allowed to propose blocks")
			return
		}

		m.chain.mu.Lock()
		err := m.chain.proposeBlock()
		m.chain.mu.Unlock()

		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "propose_failed", err.Error())
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
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		// Leader gating: only the configured leader node may propose+broadcast.
		if m.nodeID != "node1" {
			writeAPIError(w, r, http.StatusForbidden, "not_leader", "only node1 is allowed to propose+broadcast blocks")
			return
		}

		m.chain.mu.Lock()
		err := m.chain.proposeBlock()
		if err != nil {
			m.chain.mu.Unlock()
			writeAPIError(w, r, http.StatusBadRequest, "propose_failed", err.Error())
			return
		}
		b := m.chain.blocks[m.chain.height]
		_ = b
		m.chain.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// DEBUG: identity inspection
	mux.HandleFunc("/debug/nodeid", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node_id": m.nodeID,
			"id":      m.id,
		})

	})

	// DEBUG: finality inspection (visibility only)
	// GET /debug/finality
	// GET /debug/finality?h=12   (includes block finalized flag for that height if present)
	mux.HandleFunc("/debug/finality", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
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
