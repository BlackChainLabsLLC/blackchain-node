package mesh

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type MeshConfig struct {
	TLS                      *MeshTLS          `json:"tls,omitempty"`
	NodeID                   string            `json:"node_id"`
	PersistDir               string            `json:"persist_dir"`
	DataDir                  string            `json:"data_dir"`
	Listen                   string            `json:"listen"` // mesh TCP
	Host                     string            `json:"host"`
	Port                     int               `json:"port"`
	Debug                    string            `json:"debug"`       // legacy debug (deprecated)
	HttpListen               string            `json:"http_listen"` // HTTP API (REAL)
	HttpRateLimitEnabled     bool              `json:"http_rate_limit_enabled"`
	HttpRateLimitRPS         int               `json:"http_rate_limit_rps"`
	HttpRateLimitBurst       int               `json:"http_rate_limit_burst"`
	DebugEndpointsEnabled    *bool             `json:"debug_endpoints_enabled,omitempty"`
	AdminEndpointsEnabled    *bool             `json:"admin_endpoints_enabled,omitempty"`
	AllowRuntimePeerMutation bool              `json:"allow_runtime_peer_mutation"`
	Peers                    []string          `json:"peers"`
	PeerAPI                  map[string]string `json:"peer_api,omitempty"`

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

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mesh config: %w", err)
	}

	if isBootstrapConfig(raw) {
		if err := validateBootstrapConfig(raw); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("config validation: bootstrap config cannot be used as a mesh node config")
	}

	var cfg MeshConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mesh config: %w", err)
	}

	if err := validateRequiredMeshConfigFields(raw); err != nil {
		return nil, err
	}

	if err := normalizeAndValidateMeshConfig(&cfg, raw); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateRequiredMeshConfigFields(raw map[string]json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("config validation: empty config")
	}
	if _, ok := raw["node_id"]; !ok {
		return fmt.Errorf("config validation: missing required field: node_id")
	}
	if _, ok := raw["http_listen"]; !ok {
		if _, legacy := raw["debug"]; legacy {
			return fmt.Errorf("config validation: missing required field: http_listen (legacy debug is not supported; use http_listen)")
		}
		return fmt.Errorf("config validation: missing required field: http_listen")
	}
	if _, ok := raw["listen"]; ok {
		return nil
	}
	if _, ok := raw["host"]; !ok {
		return fmt.Errorf("config validation: missing required field: listen or host+port")
	}
	if _, ok := raw["port"]; !ok {
		return fmt.Errorf("config validation: missing required field: listen or host+port")
	}
	return nil
}

func isBootstrapConfig(raw map[string]json.RawMessage) bool {
	if _, ok := raw["bootstrap_peers"]; !ok {
		return false
	}
	_, hasNodeID := raw["node_id"]
	_, hasListen := raw["listen"]
	_, hasHTTPListen := raw["http_listen"]
	return !hasNodeID && !hasListen && !hasHTTPListen
}

func validateBootstrapConfig(raw map[string]json.RawMessage) error {
	var cfg bootstrapConfig
	if err := json.Unmarshal(raw["bootstrap_peers"], &cfg.BootstrapPeers); err != nil {
		return fmt.Errorf("config validation: invalid bootstrap_peers: %w", err)
	}
	for _, peer := range cfg.BootstrapPeers {
		p := strings.TrimSpace(peer)
		if p == "" {
			return fmt.Errorf("config validation: bootstrap_peers must not contain empty entries")
		}
		if _, err := validateTCPAddress("bootstrap_peer", p); err != nil {
			return err
		}
	}
	return nil
}

func normalizeAndValidateMeshConfig(cfg *MeshConfig, raw map[string]json.RawMessage) error {
	cfg.NodeID = strings.TrimSpace(cfg.NodeID)
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Debug = strings.TrimSpace(cfg.Debug)
	cfg.HttpListen = strings.TrimSpace(cfg.HttpListen)
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	cfg.PersistDir = strings.TrimSpace(cfg.PersistDir)
	if cfg.TLS != nil {
		cfg.TLS.CertFile = strings.TrimSpace(cfg.TLS.CertFile)
		cfg.TLS.KeyFile = strings.TrimSpace(cfg.TLS.KeyFile)
		cfg.TLS.CAFile = strings.TrimSpace(cfg.TLS.CAFile)
	}

	if hasRawField(raw, "listen") && (hasRawField(raw, "host") || hasRawField(raw, "port")) {
		return fmt.Errorf("config validation: listen and host/port are mutually exclusive; set only one form")
	}
	if hasRawField(raw, "host") != hasRawField(raw, "port") {
		return fmt.Errorf("config validation: host and port must be set together when listen is omitted")
	}
	if cfg.Debug != "" {
		return fmt.Errorf("config validation: legacy debug field is not supported; use http_listen plus debug_endpoints_enabled/admin_endpoints_enabled")
	}

	// Canonical mesh TCP bind
	if cfg.Listen == "" && cfg.Host != "" && cfg.Port != 0 {
		cfg.Listen = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	}

	return validateMeshConfig(cfg)
}

func hasRawField(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

func validateMeshConfig(cfg *MeshConfig) error {

	if cfg.NodeID == "" {
		return fmt.Errorf("config validation: node_id must not be empty")
	}
	if cfg.Listen == "" {
		return fmt.Errorf("config validation: listen must not be empty")
	}
	if cfg.HttpListen == "" {
		return fmt.Errorf("config validation: http_listen must not be empty")
	}
	if _, err := validateTCPAddress("listen", cfg.Listen); err != nil {
		return err
	}
	if _, err := validateTCPAddress("http_listen", cfg.HttpListen); err != nil {
		return err
	}
	if sameSocket(cfg.Listen, cfg.HttpListen) {
		return fmt.Errorf("config validation: listen and http_listen must not use the same address: %s", cfg.Listen)
	}
	debugEnabled := resolveHTTPSurfaceEnabled(cfg.DebugEndpointsEnabled, cfg.HttpListen)
	adminEnabled := resolveHTTPSurfaceEnabled(cfg.AdminEndpointsEnabled, cfg.HttpListen)
	if !isLoopbackHTTPListen(cfg.HttpListen) {
		if debugEnabled {
			return fmt.Errorf("config validation: debug_endpoints_enabled requires loopback-only http_listen; got %s", cfg.HttpListen)
		}
		if adminEnabled {
			return fmt.Errorf("config validation: admin_endpoints_enabled requires loopback-only http_listen; got %s", cfg.HttpListen)
		}
	}
	if cfg.TLS != nil {
		if err := cfg.TLS.validate(); err != nil {
			return err
		}
	}

	seen := make(map[string]struct{}, len(cfg.Peers))
	selfAddrs := candidateSelfAddresses(cfg)
	for _, peer := range cfg.Peers {
		p := strings.TrimSpace(peer)
		if p == "" {
			return fmt.Errorf("config validation: peers must not contain empty entries")
		}
		normalized, err := validateTCPAddress("peer", p)
		if err != nil {
			return err
		}
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("config validation: duplicate peer address: %s", p)
		}
		if _, ok := selfAddrs[normalized]; ok {
			return fmt.Errorf("config validation: peer list must not contain self address: %s", p)
		}
		seen[normalized] = struct{}{}
	}
	for meshAddr, apiAddr := range cfg.PeerAPI {
		meshNormalized, err := validateTCPAddress("peer_api mesh address", strings.TrimSpace(meshAddr))
		if err != nil {
			return err
		}
		if _, err := validateTCPAddress("peer_api api address", strings.TrimSpace(apiAddr)); err != nil {
			return err
		}
		if _, ok := selfAddrs[meshNormalized]; ok {
			return fmt.Errorf("config validation: peer_api must not contain self mesh address: %s", meshAddr)
		}
	}

	return nil
}

func validateNonWildcardListen(field, addr string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("config validation: invalid %s address %q: %w", field, addr, err)
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return fmt.Errorf("config validation: %s must not use a wildcard bind: %s", field, addr)
	}
	return nil
}

func validateTCPAddress(field, addr string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", fmt.Errorf("config validation: invalid %s address %q: %w", field, addr, err)
	}
	if port == "" {
		return "", fmt.Errorf("config validation: invalid %s address %q: missing port", field, addr)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", fmt.Errorf("config validation: invalid %s address %q: port must be 1-65535", field, addr)
	}
	if host != "" {
		if ip := net.ParseIP(host); ip == nil {
			if !isValidHostname(host) {
				return "", fmt.Errorf("config validation: invalid %s address %q: invalid host", field, addr)
			}
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(p)), nil
}

func candidateSelfAddresses(cfg *MeshConfig) map[string]struct{} {
	out := map[string]struct{}{}
	if cfg.Listen != "" {
		if normalized, err := validateTCPAddress("listen", cfg.Listen); err == nil {
			out[normalized] = struct{}{}
		}
	}
	if strings.TrimSpace(cfg.Host) != "" && cfg.Port != 0 {
		if normalized, err := validateTCPAddress("host+port", net.JoinHostPort(strings.TrimSpace(cfg.Host), strconv.Itoa(cfg.Port))); err == nil {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func sameSocket(a, b string) bool {
	an, errA := validateTCPAddress("address", a)
	bn, errB := validateTCPAddress("address", b)
	return errA == nil && errB == nil && an == bn
}

func isValidHostname(host string) bool {
	if host == "" {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		for i, r := range label {
			isAlphaNum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
			if !isAlphaNum && r != '-' {
				return false
			}
			if (i == 0 || i == len(label)-1) && r == '-' {
				return false
			}
		}
	}
	return true
}

// MeshTLS configures mutual-TLS for mesh TCP.
type MeshTLS struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	CAFile   string `json:"ca_file"`
}
