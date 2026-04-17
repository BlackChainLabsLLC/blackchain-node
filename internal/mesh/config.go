package mesh

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if err := cfg.normalizeAndValidate(); err != nil {
		return nil, fmt.Errorf("invalid mesh config %s: %w", path, err)
	}

	return &cfg, nil
}

func (cfg *MeshConfig) normalizeAndValidate() error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}

	cfg.Debug = strings.TrimSpace(cfg.Debug)
	cfg.HttpListen = strings.TrimSpace(cfg.HttpListen)
	if cfg.HttpListen == "" {
		if cfg.Debug != "" {
			cfg.HttpListen = cfg.Debug
		} else {
			cfg.HttpListen = "127.0.0.1:6060"
		}
	}
	if cfg.Debug != "" && cfg.Debug != cfg.HttpListen {
		return fmt.Errorf("debug (%q) and http_listen (%q) conflict; set only one or make them identical", cfg.Debug, cfg.HttpListen)
	}
	if err := validateNonWildcardListen("http_listen", &cfg.HttpListen); err != nil {
		return err
	}

	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	if cfg.Listen != "" && (cfg.Host != "" || cfg.Port != 0) {
		return fmt.Errorf("listen conflicts with host/port; use either listen or host+port")
	}
	if (cfg.Host == "") != (cfg.Port == 0) {
		return fmt.Errorf("host and port must be set together")
	}
	if cfg.Listen == "" {
		if cfg.Host != "" && cfg.Port != 0 {
			cfg.Listen = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		} else {
			cfg.Listen = "127.0.0.1:7070"
		}
	}
	if err := validateListenAddr("listen", cfg.Listen); err != nil {
		return err
	}

	for i, p := range cfg.Peers {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := validateListenAddr(fmt.Sprintf("peers[%d]", i), p); err != nil {
			return err
		}
		cfg.Peers[i] = p
	}

	if cfg.TLS != nil {
		if err := cfg.TLS.validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateNonWildcardListen(field string, addr *string) error {
	if addr == nil {
		return fmt.Errorf("%s is nil", field)
	}
	normalized := normalizeLoopbackBind(strings.TrimSpace(*addr))
	if err := validateListenAddr(field, normalized); err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(normalized)
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" || host == "0.0.0.0" || host == "::" {
		return fmt.Errorf("%s must not bind wildcard interfaces (%q); use explicit loopback or interface IP", field, normalized)
	}
	*addr = normalized
	return nil
}

func validateListenAddr(field, addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("%s is empty", field)
	}
	if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
		return fmt.Errorf("%s (%q) is not a valid tcp address: %w", field, addr, err)
	}
	return nil
}

func normalizeLoopbackBind(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return addr
	}
	if strings.TrimSpace(host) == "" {
		return net.JoinHostPort("127.0.0.1", port)
	}
	return addr
}

func validateRuntimePaths(dataDir, persistDir string) error {
	if err := ensureDirAccessible("data_dir", dataDir, true); err != nil {
		return err
	}
	if strings.TrimSpace(persistDir) != "" {
		if err := ensureDirAccessible("persist_dir", persistDir, true); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirAccessible(field, p string, create bool) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return fmt.Errorf("%s is empty", field)
	}
	p = filepath.Clean(p)
	st, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			if !create {
				return fmt.Errorf("%s does not exist: %s", field, p)
			}
			if err := os.MkdirAll(p, 0o755); err != nil {
				return fmt.Errorf("create %s (%s): %w", field, p, err)
			}
			st, err = os.Stat(p)
			if err != nil {
				return fmt.Errorf("stat %s (%s): %w", field, p, err)
			}
		} else {
			return fmt.Errorf("stat %s (%s): %w", field, p, err)
		}
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", field, p)
	}
	probe := filepath.Join(p, ".blackchain_write_probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("%s not writable (%s): %w", field, p, err)
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return nil
}

// MeshTLS configures mutual-TLS for mesh TCP.
type MeshTLS struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	CAFile   string `json:"ca_file"`
}

func (t *MeshTLS) validate() error {
	if t == nil {
		return nil
	}
	t.CertFile = strings.TrimSpace(t.CertFile)
	t.KeyFile = strings.TrimSpace(t.KeyFile)
	t.CAFile = strings.TrimSpace(t.CAFile)

	if !t.Enabled {
		if t.CertFile != "" || t.KeyFile != "" || t.CAFile != "" {
			return fmt.Errorf("tls material provided while tls.enabled=false; either enable tls or remove cert_file/key_file/ca_file")
		}
		return nil
	}
	if t.CertFile == "" || t.KeyFile == "" || t.CAFile == "" {
		return fmt.Errorf("tls enabled but cert_file/key_file/ca_file not fully configured")
	}
	if err := validateTLSPaths(t); err != nil {
		return err
	}
	if _, err := tlsConfigFrom(t, true); err != nil {
		return fmt.Errorf("tls material invalid: %w", err)
	}
	return nil
}
