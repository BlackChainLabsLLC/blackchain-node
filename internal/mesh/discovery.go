package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type discoveryBeacon struct {
	V      int    `json:"v"`
	ID     string `json:"id"`
	Listen string `json:"listen"` // dialable mesh tcp listen addr (e.g. 192.168.1.10:56211)
	Time   int64  `json:"time"`
}

type discoveredPeer struct {
	Addr      string    `json:"addr"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	ViaIP     string    `json:"via_ip"`
	Count     int       `json:"count"`
}

type discoveryConfig struct {
	Enabled bool

	UDPPort       int
	ListenAddr    string        // advertise this addr in beacons (usually cfg.Listen)
	AnnounceEvery time.Duration // leader announce cadence
	LogEvery      time.Duration
	MaxPeers      int
	AllowCIDR     string

	Persist     bool
	PersistFile string

	// Leader/Client mode (same-host safe):
	// If LeaderURL set OR DisableUDP true => client mode (no UDP bind)
	DisableUDP bool
	LeaderURL  string
	PollEvery  time.Duration

	// derived
	IsLeader bool
}

func (m *meshDaemon) enableDiscoveryIfConfigured(ctx context.Context, cfg *MeshConfig) {
	if cfg == nil || !cfg.DiscoveryEnabled {
		return
	}

	c := discoveryConfig{
		Enabled:       true,
		UDPPort:       cfg.DiscoveryUDPPort,
		ListenAddr:    strings.TrimSpace(cfg.Listen),
		AnnounceEvery: time.Duration(cfg.DiscoveryAnnounceEveryMs) * time.Millisecond,
		LogEvery:      time.Duration(cfg.DiscoveryLogEveryMs) * time.Millisecond,
		MaxPeers:      cfg.DiscoveryMaxPeers,
		AllowCIDR:     strings.TrimSpace(cfg.DiscoveryAllowCIDR),
		Persist:       cfg.DiscoveryPersist,
		PersistFile:   strings.TrimSpace(cfg.DiscoveryPersistFile),
		DisableUDP:    cfg.DiscoveryDisableUDP,
		LeaderURL:     strings.TrimSpace(cfg.DiscoveryLeaderURL),
		PollEvery:     time.Duration(cfg.DiscoveryPollEveryMs) * time.Millisecond,
	}

	// defaults
	if c.UDPPort == 0 {
		c.UDPPort = 9797
	}
	if c.AnnounceEvery <= 0 {
		c.AnnounceEvery = 900 * time.Millisecond
	}
	if c.LogEvery <= 0 {
		c.LogEvery = 5 * time.Second
	}
	if c.MaxPeers <= 0 {
		c.MaxPeers = 256
	}
	if c.PollEvery <= 0 {
		c.PollEvery = 1200 * time.Millisecond
	}

	// leader logic
	c.IsLeader = (c.LeaderURL == "" && !c.DisableUDP)

	m.discoMu.Lock()
	m.discoCfg = c
	if m.discoPeers == nil {
		m.discoPeers = make(map[string]discoveredPeer)
	}
	m.discoMu.Unlock()

	log.Printf("[discovery] enabled udp_port=%d announce_every=%s allow_cidr=%q persist=%v max_peers=%d log_every=%s leader=%v leader_url=%q poll_every=%s",
		c.UDPPort, c.AnnounceEvery, c.AllowCIDR, c.Persist, c.MaxPeers, c.LogEvery, c.IsLeader, c.LeaderURL, c.PollEvery)

	if c.Persist {
		m.discoveryLoadPersisted()
	}

	// client mode: poll leader
	if !c.IsLeader {
		go m.discoveryClientLoop(ctx)
		return
	}

	// leader mode: UDP loop
	go m.discoveryLeaderLoop(ctx)
}

func (m *meshDaemon) discoveryPersistPath() string {
	m.discoMu.RLock()
	defer m.discoMu.RUnlock()
	if strings.TrimSpace(m.discoCfg.PersistFile) != "" {
		return m.discoCfg.PersistFile
	}
	// default: allow config to point to per-node tmp (caller can set)
	return "/tmp/blackchain_discovery_peers.json"
}

func (m *meshDaemon) discoverySnapshot() []discoveredPeer {
	m.discoMu.RLock()
	defer m.discoMu.RUnlock()
	out := make([]discoveredPeer, 0, len(m.discoPeers))
	for _, p := range m.discoPeers {
		out = append(out, p)
	}
	return out
}

func (m *meshDaemon) discoveryConfigSnapshot() discoveryConfig {
	m.discoMu.RLock()
	defer m.discoMu.RUnlock()
	return m.discoCfg
}

func (m *meshDaemon) discoveryStatus() map[string]any {
	cfg := m.discoveryConfigSnapshot()
	peers := m.discoverySnapshot()
	return map[string]any{
		"enabled":        cfg.Enabled,
		"udp_port":       cfg.UDPPort,
		"listen_addr":    cfg.ListenAddr,
		"announce_every": cfg.AnnounceEvery.String(),
		"allow_cidr":     cfg.AllowCIDR,
		"persist":        cfg.Persist,
		"persist_file":   m.discoveryPersistPath(),
		"max_peers":      cfg.MaxPeers,
		"log_every":      cfg.LogEvery.String(),
		"leader":         cfg.IsLeader,
		"leader_url":     cfg.LeaderURL,
		"poll_every":     cfg.PollEvery.String(),
		"peers_count":    len(peers),
	}
}

func (m *meshDaemon) handleDiscoveryStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m.discoveryStatus())
}

func (m *meshDaemon) handleDiscoveryPeers(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"peers": m.discoverySnapshot()})
}

func (m *meshDaemon) handleDiscoveryRegister(w http.ResponseWriter, r *http.Request) {
	// Client posts {"addr":"ip:port"} to leader
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Addr string `json:"addr"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = r.Body.Close()

	addr := strings.TrimSpace(req.Addr)
	if addr == "" {
		http.Error(w, "missing addr", http.StatusBadRequest)
		return
	}
	// naive sanity: must contain colon port
	if !strings.Contains(addr, ":") {
		http.Error(w, "bad addr", http.StatusBadRequest)
		return
	}

	via := ""
	if ra := strings.TrimSpace(r.RemoteAddr); ra != "" {
		via = ra
	}

	now := time.Now()
	m.discoMu.Lock()
	if m.discoPeers == nil {
		m.discoPeers = make(map[string]discoveredPeer)
	}
	cur, ok := m.discoPeers[addr]
	if !ok {
		cur = discoveredPeer{Addr: addr, FirstSeen: now, LastSeen: now, ViaIP: via, Count: 1}
		m.discoPeers[addr] = cur
	} else {
		cur.LastSeen = now
		cur.Count++
		if cur.FirstSeen.IsZero() {
			cur.FirstSeen = now
		}
		if via != "" {
			cur.ViaIP = via
		}
		m.discoPeers[addr] = cur
	}
	m.discoMu.Unlock()

	if m.discoveryConfigSnapshot().Persist {
		m.discoveryPersistAsync()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (m *meshDaemon) discoveryClientLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.discoveryConfigSnapshot().PollEvery):
		}

		c := m.discoveryConfigSnapshot()
		leader := strings.TrimRight(c.LeaderURL, "/")
		if leader == "" {
			continue
		}

		// register self (best-effort)
		if strings.TrimSpace(c.ListenAddr) != "" {
			body := strings.NewReader(fmt.Sprintf(`{"addr":%q}`, c.ListenAddr))
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, leader+"/discovery/register", body)
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				resp, err2 := http.DefaultClient.Do(req)
				if err2 == nil && resp != nil {
					_ = resp.Body.Close()
				}
			}
		}

		// poll peers
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, leader+"/discovery/peers", nil)
		if err != nil {
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp == nil {
			continue
		}
		var payload struct {
			Peers []discoveredPeer `json:"peers"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		_ = resp.Body.Close()

		if len(payload.Peers) == 0 {
			continue
		}
		now := time.Now()
		m.discoMu.Lock()
		if m.discoPeers == nil {
			m.discoPeers = make(map[string]discoveredPeer)
		}
		for _, p := range payload.Peers {
			a := strings.TrimSpace(p.Addr)
			if a == "" || a == m.id {
				continue
			}
			cur, ok := m.discoPeers[a]
			if !ok {
				p.Addr = a
				p.FirstSeen = now
				p.LastSeen = now
				if p.Count <= 0 {
					p.Count = 1
				}
				m.discoPeers[a] = p
				continue
			}
			cur.LastSeen = now
			cur.Count++
			if cur.FirstSeen.IsZero() {
				cur.FirstSeen = now
			}
			if p.ViaIP != "" {
				cur.ViaIP = p.ViaIP
			}
			m.discoPeers[a] = cur
		}
		m.discoMu.Unlock()
	}
}

func (m *meshDaemon) discoveryLeaderLoop(ctx context.Context) {
	c := m.discoveryConfigSnapshot()

	addr, err := net.ResolveUDPAddr("udp", ":"+strconv.Itoa(c.UDPPort))
	if err != nil {
		log.Printf("[discovery] resolve udp err: %v", err)
		return
	}

	// SAME-HOST SAFE UDP BIND (SO_REUSEADDR/SO_REUSEPORT)
	lc := net.ListenConfig{
		Control: func(network, address string, rc syscall.RawConn) error {
			var cerr error
			if err := rc.Control(func(fd uintptr) {
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1)
			}); err != nil {
				cerr = err
			}
			return cerr
		},
	}

	pc, err := lc.ListenPacket(ctx, "udp", addr.String())
	if err != nil {
		log.Printf("[discovery] listen udp err: %v", err)
		return
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		log.Printf("[discovery] listen udp err: not UDPConn")
		return
	}
	defer conn.Close()

	_ = conn.SetReadBuffer(1 << 20)
	_ = conn.SetWriteBuffer(1 << 20)

	// allowlist parser (optional)
	var allowNet *net.IPNet
	if c.AllowCIDR != "" {
		_, n, e := net.ParseCIDR(c.AllowCIDR)
		if e != nil {
			log.Printf("[discovery] bad allow_cidr=%q: %v (ignored)", c.AllowCIDR, e)
		} else {
			allowNet = n
		}
	}

	seenMu := sync.Mutex{}
	seen := map[string]time.Time{} // listenAddr -> last

	pruneSeen := func(now time.Time) {
		seenMu.Lock()
		defer seenMu.Unlock()
		for k, t := range seen {
			if now.Sub(t) > 30*time.Second {
				delete(seen, k)
			}
		}
	}

	announce := func() {
		cfg := m.discoveryConfigSnapshot()
		b := discoveryBeacon{
			V:      1,
			ID:     m.id,
			Listen: strings.TrimSpace(cfg.ListenAddr),
			Time:   time.Now().Unix(),
		}
		if b.Listen == "" {
			b.Listen = m.id
		}
		raw, _ := json.Marshal(b)

		// loopback + broadcast (safe for LAN testing; can refine later)
		targets := []string{
			"127.0.0.1:" + strconv.Itoa(cfg.UDPPort),
			"255.255.255.255:" + strconv.Itoa(cfg.UDPPort),
		}

		for _, t := range targets {
			udpAddr, _ := net.ResolveUDPAddr("udp", t)
			if udpAddr == nil {
				continue
			}
			_, _ = conn.WriteToUDP(raw, udpAddr)
		}
	}

	// receiver
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			_ = conn.SetReadDeadline(time.Now().Add(900 * time.Millisecond))
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil || n <= 0 {
				continue
			}

			var b discoveryBeacon
			if e := json.Unmarshal(buf[:n], &b); e != nil {
				continue
			}
			if b.V != 1 || strings.TrimSpace(b.Listen) == "" {
				continue
			}
			if b.Listen == m.id {
				continue
			}

			// allow rules:
			// - if allow CIDR set: sender must be inside it
			// - else: allow loopback or RFC1918 private
			if allowNet != nil {
				if raddr == nil || raddr.IP == nil || !allowNet.Contains(raddr.IP) {
					continue
				}
			} else {
				if raddr != nil && raddr.IP != nil && !raddr.IP.IsLoopback() {
					ip4 := raddr.IP.To4()
					if ip4 == nil {
						continue
					}
					if !(ip4[0] == 10 || (ip4[0] == 192 && ip4[1] == 168) || (ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31)) {
						continue
					}
				}
			}

			// dedupe
			now := time.Now()
			seenMu.Lock()
			last, ok := seen[b.Listen]
			if ok && now.Sub(last) < 800*time.Millisecond {
				seenMu.Unlock()
				continue
			}
			seen[b.Listen] = now
			seenMu.Unlock()

			m.discoveryLearnPeer(b.Listen, raddr)
		}
	}()

	telemetry := time.NewTicker(c.LogEvery)
	defer telemetry.Stop()

	t := time.NewTicker(c.AnnounceEvery)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pruneSeen(time.Now())
			announce()
		case <-telemetry.C:
			m.discoveryLogTelemetry()
		}
	}
}

func (m *meshDaemon) discoveryLearnPeer(listen string, raddr *net.UDPAddr) {
	addr := strings.TrimSpace(listen)
	if addr == "" || addr == m.id {
		return
	}

	via := ""
	if raddr != nil && raddr.IP != nil {
		via = raddr.IP.String()
	} else if raddr != nil {
		via = raddr.String()
	}

	now := time.Now()
	m.discoMu.Lock()
	if m.discoPeers == nil {
		m.discoPeers = make(map[string]discoveredPeer)
	}
	cur, ok := m.discoPeers[addr]
	if !ok {
		cur = discoveredPeer{Addr: addr, FirstSeen: now, LastSeen: now, ViaIP: via, Count: 1}
		m.discoPeers[addr] = cur
	} else {
		cur.LastSeen = now
		cur.Count++
		if cur.FirstSeen.IsZero() {
			cur.FirstSeen = now
		}
		if via != "" {
			cur.ViaIP = via
		}
		m.discoPeers[addr] = cur
	}
	m.discoMu.Unlock()

	if m.discoveryConfigSnapshot().Persist {
		m.discoveryPersistAsync()
	}
}

func (m *meshDaemon) discoveryPersistAsync() {
	go func() {
		path := m.discoveryPersistPath()
		tmp := path + ".tmp"

		m.discoMu.RLock()
		out := make([]discoveredPeer, 0, len(m.discoPeers))
		for _, p := range m.discoPeers {
			out = append(out, p)
		}
		m.discoMu.RUnlock()

		raw, _ := json.MarshalIndent(out, "", "  ")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(tmp, raw, 0o644); err != nil {
			return
		}
		_ = os.Rename(tmp, path)
	}()
}

func (m *meshDaemon) discoveryLoadPersisted() {
	path := m.discoveryPersistPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var arr []discoveredPeer
	if e := json.Unmarshal(raw, &arr); e != nil {
		return
	}

	now := time.Now()
	m.discoMu.Lock()
	if m.discoPeers == nil {
		m.discoPeers = make(map[string]discoveredPeer)
	}
	for _, p := range arr {
		if strings.TrimSpace(p.Addr) == "" || p.Addr == m.id {
			continue
		}
		if p.FirstSeen.IsZero() {
			p.FirstSeen = now
		}
		m.discoPeers[p.Addr] = p
	}
	m.discoMu.Unlock()
}

func (m *meshDaemon) discoveryLogTelemetry() {
	cfg := m.discoveryConfigSnapshot()
	peers := m.discoverySnapshot()
	log.Printf("[discovery] telemetry peers=%d udp_port=%d announce=%s persist=%v leader=%v",
		len(peers), cfg.UDPPort, cfg.AnnounceEvery, cfg.Persist, cfg.IsLeader)
}

// discoveryPromoteLoop promotes discovered peers into bootstrapPeers
func (m *meshDaemon) discoveryPromoteLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !m.discoveryConfigSnapshot().Enabled {
				continue
			}

			// snapshot discovered peers

			peers := m.discoverySnapshot()

			for _, p := range peers {
				addr := p.Addr
				if addr == "" {
					continue
				}

				// already bootstrap?
				found := false
				for _, bp := range m.bootstrapPeers {
					if bp == addr {
						found = true
						break
					}
				}
				if found {
					continue
				}

				// cap
				if len(m.bootstrapPeers) >= m.discoveryConfigSnapshot().MaxPeers {
					break
				}

				// attempt reachability
				m.ConnectToPeers([]string{addr})

				// check reachable
				if peer, ok := m.peers[addr]; ok && peer.Reachable {
					log.Printf("[discovery] promoted → %s", addr)
					m.bootstrapPeers = append(m.bootstrapPeers, addr)
				}
			}
		}
	}
}

// discoveryEvictDeadLoop removes unreachable bootstrap peers
func (m *meshDaemon) discoveryEvictDeadLoop(ctx context.Context) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			out := m.bootstrapPeers[:0]
			for _, addr := range m.bootstrapPeers {
				if peer, ok := m.peers[addr]; ok && peer.Reachable {
					out = append(out, addr)
				}
			}
			m.bootstrapPeers = out
		}
	}
}
