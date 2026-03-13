package mesh

import (
	"encoding/json"
	"os"
)

type bootstrapConfig struct {
	BootstrapPeers []string `json:"bootstrap_peers"`
}

func LoadBootstrapPeers() []string {

	file := "config/bootstrap.json"

	data, err := os.ReadFile(file)
	if err != nil {
		return []string{}
	}

	var cfg bootstrapConfig

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return []string{}
	}

	return cfg.BootstrapPeers
}
