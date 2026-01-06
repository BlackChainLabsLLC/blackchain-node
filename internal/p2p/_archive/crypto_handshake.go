package p2p

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net"
    "net/http"
    "strconv"
    "time"
)

// gossipToRPCAddr converts a gossip address like "10.200.0.3:55113"
// into the corresponding RPC address "10.200.0.3:55213".
func gossipToRPCAddr(gossipAddr string) (string, error) {
    host, portStr, err := net.SplitHostPort(gossipAddr)
    if err != nil {
        return "", err
    }
    port, err := strconv.Atoi(portStr)
    if err != nil {
        return "", err
    }
    rpcPort := port + 100
    return fmt.Sprintf("%s:%d", host, rpcPort), nil
}

// cryptoHandshakeOnce performs a single handshake pass against all static peers.
func (n *Node) cryptoHandshakeOnce() {
    if n.Crypto == nil {
        return
    }

    // Snapshot static peers under lock.
    n.mu.Lock()
    peers := make([]string, len(n.staticPeers))
    copy(peers, n.staticPeers)
    n.mu.Unlock()

    if len(peers) == 0 {
        return
    }

    for _, gossipAddr := range peers {
        rpcAddr, err := gossipToRPCAddr(gossipAddr)
        if err != nil {
            log.Printf("crypto handshake: bad peer addr %q: %v", gossipAddr, err)
            continue
        }

        payload := map[string]string{
            "id":     n.ID,
            "pubkey": n.Crypto.SelfPublicKey,
        }

        buf := &bytes.Buffer{}
        if err := json.NewEncoder(buf).Encode(payload); err != nil {
            log.Printf("crypto handshake: encode error for %s: %v", rpcAddr, err)
            continue
        }

        resp, err := http.Post("http://"+rpcAddr+"/crypto/handshake", "application/json", buf)
        if err != nil {
            log.Printf("crypto handshake: POST to %s failed: %v", rpcAddr, err)
            continue
        }

        var out struct {
            ID     string `json:"id"`
            PubKey string `json:"pubkey"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
            log.Printf("crypto handshake: decode from %s failed: %v", rpcAddr, err)
            resp.Body.Close()
            continue
        }
        resp.Body.Close()

        if out.ID == "" || out.PubKey == "" {
            continue
        }

        if err := n.Crypto.RegisterPeerPublicKey(out.ID, out.PubKey); err != nil {
            log.Printf("crypto handshake: register peer %s failed: %v", out.ID, err)
            continue
        }

        log.Printf("crypto handshake: established pubkey with peer %s (%s)", out.ID, rpcAddr)
    }
}

// startCryptoHandshakeLoop periodically runs cryptoHandshakeOnce.
// It is started as a goroutine from Node.Run().
func (n *Node) startCryptoHandshakeLoop() {
    // Small initial delay to give peers time to start their RPC listeners.
    time.Sleep(2 * time.Second)

    for {
        n.cryptoHandshakeOnce()
        time.Sleep(30 * time.Second)
    }
}

