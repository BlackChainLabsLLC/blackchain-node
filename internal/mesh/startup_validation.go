package mesh

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

const maxStartupPeers = 256

// validateStartupConfig enforces fail-closed startup checks for critical network wiring.
func validateStartupConfig(cfg *MeshConfig) error {
	if cfg == nil {
		return fmt.Errorf("missing mesh config")
	}

	if err := validateListenAddr(cfg.Listen, true); err != nil {
		return fmt.Errorf("invalid listen address %q: %w", cfg.Listen, err)
	}
	if err := validateListenAddr(cfg.HttpListen, true); err != nil {
		return fmt.Errorf("invalid http_listen address %q: %w", cfg.HttpListen, err)
	}

	if canonicalAddr(cfg.Listen) == canonicalAddr(cfg.HttpListen) {
		return fmt.Errorf("listen and http_listen must not be identical: %q", cfg.Listen)
	}

	if len(cfg.Peers) > maxStartupPeers {
		return fmt.Errorf("too many configured peers: %d > %d", len(cfg.Peers), maxStartupPeers)
	}

	seen := make(map[string]struct{}, len(cfg.Peers))
	for i, peer := range cfg.Peers {
		peer = strings.TrimSpace(peer)
		if peer == "" {
			continue
		}
		if err := validatePeerAddr(peer); err != nil {
			return fmt.Errorf("invalid peer[%d] %q: %w", i, peer, err)
		}
		canon := canonicalAddr(peer)
		if canon == canonicalAddr(cfg.Listen) {
			return fmt.Errorf("peer[%d] %q points to local listen address", i, peer)
		}
		if _, ok := seen[canon]; ok {
			return fmt.Errorf("duplicate peer address %q", peer)
		}
		seen[canon] = struct{}{}
	}

	return nil
}

func normalizeBootstrapPeers(peers []string, localListen string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(peers))
	local := canonicalAddr(localListen)

	for i, peer := range peers {
		peer = strings.TrimSpace(peer)
		if peer == "" {
			continue
		}
		if err := validatePeerAddr(peer); err != nil {
			return nil, fmt.Errorf("invalid bootstrap_peers[%d] %q: %w", i, peer, err)
		}
		canon := canonicalAddr(peer)
		if canon == local {
			return nil, fmt.Errorf("bootstrap peer %q points to local listen address", peer)
		}
		if _, ok := seen[canon]; ok {
			continue
		}
		seen[canon] = struct{}{}
		out = append(out, peer)
	}

	sort.Strings(out)
	return out, nil
}

func validatePeerAddr(addr string) error {
	if err := validateListenAddr(addr, false); err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(addr)
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("peer host must be non-empty")
	}
	return nil
}

func validateListenAddr(addr string, allowEmptyHost bool) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	if !allowEmptyHost && strings.TrimSpace(host) == "" {
		return fmt.Errorf("host must be non-empty")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return fmt.Errorf("port must be in 1..65535")
	}
	return nil
}

func canonicalAddr(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}
