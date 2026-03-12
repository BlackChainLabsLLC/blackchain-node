package mesh

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
)

type MeshConfig struct {
	TLS                  *MeshTLS `json:"tls,omitempty"`
	NodeID               string   `json:"node_id"`
	PersistDir           string   `json:"persist_dir"`
	DataDir              string   `json:"data_dir"`
	Listen               string   `json:"listen"` // mesh TCP
	Host                 string   `json:"host"`
	Port                 int      `json:"port"`
	Debug                string   `json:"debug"`       // legacy debug (deprecated)
	HttpListen           string   `json:"http_listen"` // HTTP API (REAL)
	HttpRateLimitEnabled bool     `json:"http_rate_limit_enabled"`
	HttpRateLimitRPS     int      `json:"http_rate_limit_rps"`
	HttpRateLimitBurst   int      `json:"http_rate_limit_burst"`
	Peers                []string `json:"peers"`

	// Discovery (UDP leader/client)
	DiscoveryEnabled         bool   `json:"discovery_enabled"`
	DiscoveryUDPPort         int    `json:"discovery_udp_port"`
	DiscoveryAnnounceEveryMs int    `json:"discovery_announce_every_ms"`
	DiscoveryLogEveryMs      int    `json:"discovery_log_every_ms"`
	DiscoveryMaxPeers        int    `json:"discovery_max_peers"`
	DiscoveryAllowCIDR       string `json:"discovery_allow_cidr"`
	DiscoveryPersist         bool   `json:"discovery_persist"`
	DiscoveryPersistFile     string `json:"discovery_persist_file"`
	DiscoveryDisableUDP      bool   `json:"discovery_disable_udp"`
	DiscoveryLeaderURL       string `json:"discovery_leader_url"`
	DiscoveryPollEveryMs     int    `json:"discovery_poll_every_ms"`
}

func LoadMeshConfig(path string) (*MeshConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mesh config: %w", err)
	}

	var cfg MeshConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mesh config: %w", err)
	}

	// Canonical HTTP API bind
	if cfg.HttpListen == "" {
		if cfg.Debug != "" {
			cfg.HttpListen = cfg.Debug
		} else {
			cfg.HttpListen = ":6060"
		}
	}

	// Canonical mesh TCP bind
	if cfg.Listen == "" {
		if cfg.Host != "" && cfg.Port != 0 {
			cfg.Listen = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		} else {
			cfg.Listen = "127.0.0.1:7070"
		}
	}

	return &cfg, nil
}

// MeshTLS configures mutual-TLS for mesh TCP.
type MeshTLS struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	CAFile   string `json:"ca_file"`
}
