package p2p

import (
    "encoding/json"
    "log"
    "os"
    "path/filepath"
    "time"

    "blackchain/internal/store"
)

const peersFilePath = "data/peers.json"

type persistedPeer struct {
    ID       string  `json:"id"`
    Address  string  `json:"addr"`
    LastSeen int64   `json:"last_seen"`
    Score    float64 `json:"score"`
}

// SavePeersToDisk serializes the current peer set to disk.
func (n *Node) SavePeersToDisk() {
    peers := n.Store.All()
    list := make([]persistedPeer, 0, len(peers))
    for _, p := range peers {
        list = append(list, persistedPeer{
            ID:       p.ID,
            Address:  p.Address,
            LastSeen: p.LastSeen,
            Score:    p.Score,
        })
    }

    if err := os.MkdirAll(filepath.Dir(peersFilePath), 0o755); err != nil {
        log.Printf("peer-save mkdir error: %v", err)
        return
    }

    f, err := os.Create(peersFilePath)
    if err != nil {
        log.Printf("peer-save create error: %v", err)
        return
    }
    defer f.Close()

    enc := json.NewEncoder(f)
    if err := enc.Encode(list); err != nil {
        log.Printf("peer-save encode error: %v", err)
        return
    }
}

// LoadPeersFromDisk loads peers from disk into the store and staticPeers.
func (n *Node) LoadPeersFromDisk() {
    f, err := os.Open(peersFilePath)
    if err != nil {
        if !os.IsNotExist(err) {
            log.Printf("peer-load open error: %v", err)
        }
        return
    }
    defer f.Close()

    var list []persistedPeer
    dec := json.NewDecoder(f)
    if err := dec.Decode(&list); err != nil {
        log.Printf("peer-load decode error: %v", err)
        return
    }

    now := time.Now().Unix()
    for _, p := range list {
        // Skip obviously ancient peers (> 5 minutes old)
        if now-p.LastSeen > 300 {
            continue
        }

        n.Store.Upsert(&store.Peer{
            ID:       p.ID,
            Address:  p.Address,
            LastSeen: p.LastSeen,
            Score:    p.Score,
        })

        if p.Address != "" && p.Address != n.gossipAddr() {
            _ = n.AddStaticPeer(p.Address)
        }
    }
}

// pruneStalePeersLoop periodically drops old peers and saves current view.
func (n *Node) pruneStalePeersLoop() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        cutoff := time.Now().Unix() - 300 // 5 minutes
        removed := n.Store.PruneOlderThan(cutoff)
        if removed > 0 {
            log.Printf("peer store: pruned %d stale peers", removed)
        }
        n.SavePeersToDisk()
    }
}

