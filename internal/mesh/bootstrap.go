package mesh

import (
	"encoding/json"
	"fmt"
	"os"
)

type bootstrapConfig struct {
	BootstrapPeers []string `json:"bootstrap_peers"`
}

func LoadBootstrapPeers() []string {
	peers, err := loadBootstrapPeersFromFile("config/bootstrap.json")
	if err != nil {
		return []string{}
	}
	return peers
}

func loadBootstrapPeersFromFile(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read bootstrap config: %w", err)
	}

	var cfg bootstrapConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse bootstrap config: %w", err)
	}
	return cfg.BootstrapPeers, nil
}
