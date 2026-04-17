package mesh

import (
	"fmt"
	"strings"
)

func (m *meshDaemon) requireValidatorActionReady(action string) error {
	if strings.TrimSpace(m.nodeID) != "node1" {
		return fmt.Errorf("%s denied: node %q is not leader", action, m.nodeID)
	}

	m.chain.mu.Lock()
	defer m.chain.mu.Unlock()

	id, err := m.chain.EnsureValidatorIdentityLocked()
	if err != nil {
		return fmt.Errorf("%s denied: validator identity invalid: %w", action, err)
	}
	if id == "" || id == "ERR_NO_VALIDATOR" {
		return fmt.Errorf("%s denied: validator identity unavailable", action)
	}
	return nil
}
