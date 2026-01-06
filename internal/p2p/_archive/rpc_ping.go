package p2p

import (
    "encoding/json"
    "net/http"
    "time"
)

type pingResponse struct {
    Pong      bool   `json:"pong"`
    Timestamp int64  `json:"ts"`
    NodeID    string `json:"node_id"`
}

func (n *Node) handlePing(w http.ResponseWriter, r *http.Request) {
    resp := pingResponse{
        Pong:      true,
        Timestamp: time.Now().Unix(),
        NodeID:    n.ID,
    }
    data, _ := json.Marshal(resp)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    w.Write(data)
}
