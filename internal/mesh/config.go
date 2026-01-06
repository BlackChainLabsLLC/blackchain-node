package mesh

import (
	"encoding/json"
	"fmt"
	"os"
)

type MeshConfig struct {
	Listen string   `json:"listen"`
	Debug  string   `json:"debug"` // debug HTTP API port, must be unique per node
	Peers  []string `json:"peers"`
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

	// Default debug port if not provided
	if cfg.Debug == "" {
		cfg.Debug = ":6060"
	}

	return &cfg, nil
}
