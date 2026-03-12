package mesh

import (
	"encoding/json"
	"io"
	"net/http"
)

// registerSnapshotHandlers exposes snapshot endpoints for fast sync.
func (m *meshDaemon) registerSnapshotHandlers(mux *http.ServeMux) {

	// GET /chain/snapshot
	mux.HandleFunc("/chain/snapshot", func(w http.ResponseWriter, _ *http.Request) {
		m.chain.mu.RLock()
		snap := m.chain.ExportSnapshotLocked()
		m.chain.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})

	// POST /chain/snapshot/apply
	mux.HandleFunc("/chain/snapshot/apply", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		m.chain.mu.Lock()
		ok, err := m.chain.LoadSnapshotFromBytes(raw)
		m.chain.mu.Unlock()

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": ok,
		})
	})
}
