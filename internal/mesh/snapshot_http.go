package mesh

import (
	"encoding/json"
	"net/http"
)

// registerSnapshotHandlers exposes snapshot endpoints for fast sync.
func (m *meshDaemon) registerSnapshotHandlers(mux *http.ServeMux) {

	// GET /chain/snapshot
	mux.HandleFunc("/chain/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		m.chain.mu.RLock()
		snap := m.chain.ExportSnapshotLocked()
		m.chain.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})

	// POST /chain/snapshot/apply
	mux.HandleFunc("/chain/snapshot/apply", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		raw, err := readBodyBytes(w, r, maxSnapshotBodyBytes)
		if err != nil {
			writeAPIError(r, w, http.StatusBadRequest, "invalid_snapshot_body", err.Error())
			return
		}

		m.chain.mu.Lock()
		ok, err := m.chain.LoadSnapshotFromBytes(raw)
		m.chain.mu.Unlock()

		if err != nil {
			writeAPIError(r, w, http.StatusBadRequest, "snapshot_rejected", err.Error())
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": ok,
		})
	})
}
