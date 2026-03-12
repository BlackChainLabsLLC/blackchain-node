package mesh

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// StateHashLocked returns a deterministic hash of chain state (height, tip, accounts).
// Caller MUST hold c.mu (RLock/Lock ok).
//
// Format hashed:
//
//	height=<H>\n
//	tip=<TIP>\n
//	acct <addr> bal=<BAL> nonce=<NONCE>\n   (sorted by addr)
func (c *ProductionChain) StateHashLocked() string {
	var buf bytes.Buffer

	// chain header
	buf.WriteString(fmt.Sprintf("height=%d\n", c.height))
	buf.WriteString(fmt.Sprintf("tip=%s\n", c.tip))

	// accounts (sorted)
	keys := make([]string, 0, len(c.accounts))
	for k := range c.accounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, addr := range keys {
		ac := c.accounts[addr]
		if ac == nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("acct %s bal=%d nonce=%d\n", addr, ac.Balance, ac.Nonce))
	}

	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}

func (c *ProductionChain) SignedStateHash() map[string]any {
	h := c.StateHashLocked()
	sig := simpleSign(h)
	return map[string]any{
		"height":     c.height,
		"state_hash": h,
		"sig":        sig,
	}
}

func simpleSign(s string) string {
	h := sha256Sum("blackchain|" + s)
	return h
}

func sha256Sum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Phase Q: check if given height+hash matches peer quorum.
// Rule: require majority of observed peers (>= floor(total/2)+1). If no peers observed yet, allow.
func (m *meshDaemon) hasQuorum(height int64, hash string) bool {
	agree := 0
	total := 0

	m.peerStateMu.Lock()
	for _, ann := range m.peerState {
		total++
		if ann.Height == height && ann.StateHash == hash {
			agree++
		}
	}
	m.peerStateMu.Unlock()

	if total == 0 {
		return true
	}
	need := (total / 2) + 1
	return agree >= need
}
