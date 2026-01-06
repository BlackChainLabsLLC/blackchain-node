package store

import (
    "sync"
)

type Peer struct {
    ID       string
    Address  string
    LastSeen int64
    Score    float64
}

type PeerStore struct {
    mu    sync.RWMutex
    peers map[string]*Peer
}

func NewPeerStore() *PeerStore {
    return &PeerStore{
        peers: map[string]*Peer{},
    }
}

func (ps *PeerStore) Upsert(p *Peer) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    ps.peers[p.Address] = p
}

func (ps *PeerStore) All() []*Peer {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    out := []*Peer{}
    for _, p := range ps.peers {
        out = append(out, p)
    }
    return out
}

func (ps *PeerStore) BumpScore(addr string, delta float64) {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    peer, ok := ps.peers[addr]
    if !ok {
        return
    }

    peer.Score += delta
    if peer.Score < -3 {
        delete(ps.peers, addr)
    }
}

func (ps *PeerStore) PruneOlderThan(cutoff int64) int {
    ps.mu.Lock()
    defer ps.mu.Unlock()

    removed := 0
    for addr, p := range ps.peers {
        if p.LastSeen < cutoff {
            delete(ps.peers, addr)
            removed++
        }
    }
    return removed
}

