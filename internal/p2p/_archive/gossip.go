package p2p

import (
    "encoding/json"
    "log"
    "net"
    "strconv"
    "time"

    "blackchain/internal/store"
)

type gossipMessage struct {
    ID    string   `json:"id"`
    Addr  string   `json:"addr"`
    Peers []string `json:"peers"`
}

func (n *Node) runGossip() {
    addr := ":" + n.GossipPort
    conn, err := net.ListenPacket("udp", addr)
    if err != nil {
        log.Printf("gossip listen error on %s: %v", addr, err)
        return
    }
    defer conn.Close()

    log.Printf("📡 gossip listening on %s", addr)
    go n.gossipLoop(conn)

    buf := make([]byte, 2048)
    for {
        nBytes, remote, err := conn.ReadFrom(buf)
        if err != nil {
            log.Printf("gossip read error: %v", err)
            return
        }

        var msg gossipMessage
        if err := json.Unmarshal(buf[:nBytes], &msg); err != nil {
            log.Printf("⚠️ gossip decode error from %s: %v", remote.String(), err)
            continue
        }

        n.handleGossip(msg, remote)
    }
}

func (n *Node) gossipLoop(conn net.PacketConn) {
    for {
        time.Sleep(2 * time.Second)

        peers := n.StaticPeers()
        msg := gossipMessage{
            ID:    n.ID,
            Addr:  n.gossipAddr(),
            Peers: peers,
        }

        data, _ := json.Marshal(msg)

        for _, peer := range peers {
            udpAddr, err := net.ResolveUDPAddr("udp", peer)
            if err != nil {
                log.Printf("gossip resolve error for %s: %v", peer, err)
                continue
            }
            if _, err := conn.WriteTo(data, udpAddr); err != nil {
                log.Printf("gossip send error to %s: %v", peer, err)
                continue
            }
        }

        log.Printf("📡 gossip broadcasted (%d peers)", len(peers))
    }
}

func (n *Node) handleGossip(msg gossipMessage, remote net.Addr) {
    addr := msg.Addr
    if addr == "" {
        if udp, ok := remote.(*net.UDPAddr); ok {
            addr = udp.IP.String() + ":" + strconv.Itoa(udp.Port)
        } else {
            addr = remote.String()
        }
    }

    now := time.Now().Unix()

    // Always record the direct gossip peer in the peer store.
    n.Store.Upsert(&store.Peer{
        ID:       msg.ID,
        Address:  addr,
        LastSeen: now,
        Score:    1,
    })

    if n.Routes != nil {
        // Direct link: msg.ID is reachable via addr at distance 1.
        n.Routes.UpdateDirect(msg.ID, addr)

        // Use the sender"s peer list only to enrich routing knowledge,
        // NOT to expand our static peer set. This preserves the physical
        // topology defined by CLI -peer flags while still learning multi-hop paths.
        peers := make([]*store.Peer, 0, len(msg.Peers))
        for _, pAddr := range msg.Peers {
            if pAddr == "" || pAddr == addr {
                continue
            }
            peers = append(peers, &store.Peer{
                ID:       pAddr,
                Address:  pAddr,
                LastSeen: now,
                Score:    1,
            })
        }
        if len(peers) > 0 {
            n.Routes.UpdateFromPeerList(msg.ID, addr, peers)
        }
    }

    // IMPORTANT:
    // We deliberately do NOT call AddStaticPeer() here anymore.
    // Static peers are only those explicitly configured (CLI -peer flags).
    // This prevents the gossip layer from "upgrading" the topology into
    // a full mesh and keeps chain/line topologies stable for simulation.
}

