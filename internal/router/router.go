package router
import (
    "encoding/json"
    "net"
)
type Router struct {
    GossipPort string
}
func NewRouter(port string) *Router {
    return &Router{ GossipPort: port }
}
func (r *Router) Listen(handler func([]byte, *net.UDPAddr)) error {
    addr, _ := net.ResolveUDPAddr("udp", ":"+r.GossipPort)
    conn, err := net.ListenUDP("udp", addr)
    if err != nil { return err }
    buf := make([]byte, 4096)
    go func() {
        for {
            n, src, _ := conn.ReadFromUDP(buf)
            msg := buf[:n]
            handler(msg, src)
        }
    }()
    return nil
}
func (r *Router) Gossip(msg any, targets []string) {
    data, _ := json.Marshal(msg)
    for _, t := range targets {
        udpAddr, _ := net.ResolveUDPAddr("udp", t)
        conn, _ := net.DialUDP("udp", nil, udpAddr)
        conn.Write(data)
        conn.Close()
    }
}

