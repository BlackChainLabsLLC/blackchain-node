package p2p

import (
    "encoding/json"
    "net/http"
)

// Debug Suite v2 — Telemetry, State, Crypto Snapshot
func (n *Node) registerDebugHandlers(mux *http.ServeMux) {

    // /debug/v2/traffic — live msg counters
    mux.HandleFunc("/debug/v2/traffic", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{
            "id":         n.ID,
            "msgs_in":    n.MsgsIn,
            "msgs_out":   n.MsgsOut,
            "msgs_relay": n.MsgsRelay,
        })
    })

    // /debug/state — route + peer view
    mux.HandleFunc("/debug/state", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type","application/json")
        var routes []Route
        if n.Routes != nil { routes = n.Routes.Snapshot() }
        _ = json.NewEncoder(w).Encode(map[string]any{
            "id":    n.ID,
            "routes": routes,
            "peers":  n.Store.All(),
        })
    })

    // /debug/crypto — ECC view
    mux.HandleFunc("/debug/crypto", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type","application/json")
        if n.Crypto == nil {
            _ = json.NewEncoder(w).Encode(map[string]any{"enabled":false})
            return
        }
        _ = json.NewEncoder(w).Encode(map[string]any{
            "enabled":     true,
            "public_key":  n.Crypto.SelfPublicKey,
            "peers":       n.Crypto.Peers,
        })
    })
}
