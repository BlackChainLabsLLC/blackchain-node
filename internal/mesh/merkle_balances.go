package mesh

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
)

// merkleBalancesRoot computes a deterministic Merkle root over balances/nonces.
// Leaves are ordered by address ASC, and each leaf is:
//
//	H( addr || "\n" || balance || "\n" || nonce )
//
// Parent nodes are:
//
//	H( left || right )
//
// If odd number of leaves at a level, the last leaf is duplicated (standard deterministic rule).
func merkleBalancesRoot(accts map[string]*Account) string {
	type pair struct {
		addr  string
		bal   int64
		nonce int64
	}

	lst := make([]pair, 0, len(accts))
	for addr, a := range accts {
		// Treat absent and zero-value accounts as equivalent for state commitment.
		// This prevents nodes from diverging just because they pre-create local 0-balance accounts.
		if a == nil {
			continue
		}
		bal := a.Balance
		nonce := a.Nonce
		if bal == 0 && nonce == 0 {
			continue
		}
		lst = append(lst, pair{addr: addr, bal: bal, nonce: nonce})
	}

	sort.Slice(lst, func(i, j int) bool { return lst[i].addr < lst[j].addr })

	// If there are no accounts, root is hash of empty (still deterministic).
	if len(lst) == 0 {
		sum := sha256.Sum256([]byte{})
		return hex.EncodeToString(sum[:])
	}

	leaves := make([][]byte, 0, len(lst))
	for _, p := range lst {
		h := sha256.Sum256([]byte(p.addr + "\n" + strconv.FormatInt(p.bal, 10) + "\n" + strconv.FormatInt(p.nonce, 10)))
		// store raw bytes for tree combine (not hex string)
		b := make([]byte, len(h))
		copy(b, h[:])
		leaves = append(leaves, b)
	}

	// Build up the tree.
	for len(leaves) > 1 {
		next := make([][]byte, 0, (len(leaves)+1)/2)
		for i := 0; i < len(leaves); i += 2 {
			left := leaves[i]
			right := left
			if i+1 < len(leaves) {
				right = leaves[i+1]
			}
			merged := append(append([]byte{}, left...), right...)
			h := sha256.Sum256(merged)
			b := make([]byte, len(h))
			copy(b, h[:])
			next = append(next, b)
		}
		leaves = next
	}

	return hex.EncodeToString(leaves[0])
}
