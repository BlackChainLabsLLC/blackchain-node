package mesh

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const defaultBootstrapConfigPath = "config/bootstrap.json"

type bootstrapConfig struct {
	BootstrapPeers []string `json:"bootstrap_peers"`
}

func LoadBootstrapPeers() []string {
	peers, err := loadBootstrapPeersFromFile(defaultBootstrapConfigPath, nil)
	if err != nil {
		return []string{}
	}
	return peers
}

func loadBootstrapPeersFromFile(path string, selfAddrs map[string]struct{}) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("startup validation: bootstrap config path must not be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("startup validation: read bootstrap config %s: %w", path, err)
	}

	var cfg bootstrapConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("startup validation: parse bootstrap config %s: %w", path, err)
	}
	return normalizeBootstrapPeers(cfg.BootstrapPeers, selfAddrs)
}

func normalizeBootstrapPeers(peers []string, selfAddrs map[string]struct{}) ([]string, error) {
	if len(peers) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(peers))
	seen := make(map[string]struct{}, len(peers))
	for i, peer := range peers {
		p := strings.TrimSpace(peer)
		if p == "" {
			return nil, fmt.Errorf("startup validation: bootstrap peer entry %d is empty", i)
		}
		addr, err := validateTCPAddress("bootstrap_peer", p)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[addr]; ok {
			return nil, fmt.Errorf("startup validation: duplicate bootstrap peer: %s", p)
		}
		if selfAddrs != nil {
			if _, ok := selfAddrs[addr]; ok {
				return nil, fmt.Errorf("startup validation: bootstrap peer must not contain self address: %s", p)
			}
		}
		seen[addr] = struct{}{}
		normalized = append(normalized, addr)
	}

	sort.Strings(normalized)
	return normalized, nil
}
