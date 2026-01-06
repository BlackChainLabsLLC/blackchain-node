package p2p

import (
    "encoding/json"
    "net/http"
    "log"
)

// POST /send → User-facing message injection into mesh
// { "from": "ns1", "to": "ns3", "body": "HELLO" }
func (n *Node) registerSendHandler(mux *http.ServeMux) {
    mux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {

        type req struct {
            From string `json:"from"`
            To   string `json:"to"`
            Body string `json:"body"`
            Hops int    `json:"hops"`
        }

        var msg req
        if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
            http.Error(w,"bad_json",400)
            return
        }

        if msg.From == "" { msg.From = n.ID }
        if msg.Hops < 0 { msg.Hops = 0 }

        // Main send path (final API)
        if err := n.SendMessage(msg.To, msg.Body, msg.Hops); err != nil {
            log.Printf("❌ SEND FAIL: %s → %s :: %v", msg.From, msg.To, err)
            http.Error(w,"no_route",502)
            return
        }

        _ = json.NewEncoder(w).Encode(map[string]any{
            "ok":   true,
            "from": msg.From,
            "to":   msg.To,
            "body": msg.Body,
            "hops": msg.Hops,
        })
    })
}

