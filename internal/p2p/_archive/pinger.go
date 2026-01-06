package p2p

import (
    "encoding/json"
    "net/http"
    "time"
)

type pingResult struct {
    ok   bool
    addr string
}

func (n *Node) pingOnce(addr string) bool {
    client := &http.Client{Timeout: 800 * time.Millisecond}
    resp, err := client.Get("http://" + addr + "/ping")
    if err != nil {
        return false
    }
    defer resp.Body.Close()

    var out map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return false
    }
    return true
}

func (n *Node) startPinger() {
    go func() {
        for {
            time.Sleep(2 * time.Second)

            peers := n.StaticPeers()
            for _, p := range peers {
                alive := n.pingOnce(p)

                if alive {
                    n.Store.BumpScore(p, +0.5)
                } else {
                    n.Store.BumpScore(p, -1.0)
                }
            }
        }
    }()
}

