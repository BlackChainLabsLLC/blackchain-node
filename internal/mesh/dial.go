package mesh

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"
)

// ConnectToPeers performs reachability checks (dial-based)
// and sends one valid wireMessage so inbound decode can flip activity truth.
func (m *meshDaemon) ConnectToPeers(peerList []string) {
	go func() {
		for {
			for _, addr := range peerList {

				addr = strings.TrimSpace(addr)
				if addr == "" || addr == m.id {
					continue
				}

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				conn, err := meshDialTimeout(ctx, addr, 2*time.Second, m.tlsCfg)
				cancel()

				if err != nil {
					m.TouchReachable(addr, false)
					continue
				}

				m.TouchReachable(addr, true)

				hello := wireMessage{
					ID:    "hello-" + time.Now().UTC().Format("150405.000000000"),
					Type:  "msg",
					From:  m.id,
					Via:   m.id,
					To:    addr,
					Body:  "mesh-hello",
					Topic: "hello",
					Time:  time.Now().Unix(),
					TTL:   1,
				}

				if b, e := json.Marshal(hello); e == nil {
					b = append(b, '\n')
					_, _ = conn.Write(b)
				}

				_ = conn.Close()
				log.Println("[peers] reachable+hello →", addr)
			}

			time.Sleep(3 * time.Second)
		}
	}()
}
