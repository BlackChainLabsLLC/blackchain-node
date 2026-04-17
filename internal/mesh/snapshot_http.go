package mesh

import (
	"encoding/json"
	"net/http"
)

// registerSnapshotHandlers exposes snapshot endpoints for fast sync.
func (m *meshDaemon) registerSnapshotHandlers(mux *http.ServeMux) {

	// GET /chain/snapshot
	mux.HandleFunc("/chain/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
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
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		var raw json.RawMessage
		if !decodeJSONBody(w, r, &raw, maxJSONBodyLarge) {
			return
		}

		m.chain.mu.Lock()
		ok, err := m.chain.LoadSnapshotFromBytes([]byte(raw))
		m.chain.mu.Unlock()

		if err != nil {
			writeAPIError(w, r, http.StatusBadRequest, "snapshot_apply_failed", err.Error())
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": ok,
		})
	})
}
