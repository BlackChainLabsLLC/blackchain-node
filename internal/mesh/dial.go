package mesh

import (
	"log"
	"net"
	"time"
)

// ConnectToPeers performs REACHABILITY checks only.
// It NEVER bumps LastSeen or Connected (activity).
func (m *meshDaemon) ConnectToPeers(peerList []string) {
	go func() {
		for {
			for _, addr := range peerList {
				conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
				if err != nil {
					m.TouchReachable(addr, false)
					continue
				}

				_, _ = conn.Write([]byte("mesh-hello\n"))
				_ = conn.Close()

				m.TouchReachable(addr, true)
				log.Println("[peers] reachable →", addr)
			}
			time.Sleep(3 * time.Second)
		}
	}()
}
