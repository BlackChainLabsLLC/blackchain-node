// Production runtime config surface (Product Lock).
package mesh

import (
	"encoding/json"
	"os"
)

type runtimeConfig struct {
	NodeID     string `json:"node_id"`
	HTTPListen string `json:"http_listen"`
	UDPListen  string `json:"udp_listen"`
}

// loadRuntimeConfig reads config json for product endpoints.
// If missing or unreadable, returns zero values and product will still run.
func loadRuntimeConfig(path string) runtimeConfig {
	var rc runtimeConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return rc
	}
	_ = json.Unmarshal(b, &rc)
	return rc
}
