package mesh

import "time"

const (
	// SeenTTL is how long we remember message IDs to prevent loops/duplication.
	SeenTTL = 2 * time.Minute

	// SeenMax is a hard cap to prevent unbounded growth.
	SeenMax = 5000
)

// markSeen records an id as processed.
// It also prunes old entries and enforces a hard cap.
// NOTE: caller does NOT need to hold m.lock; markSeen handles locking.
func (m *meshDaemon) markSeen(id string) {
	if id == "" {
		return
	}

	now := time.Now()

	m.lock.Lock()
	defer m.lock.Unlock()

	if m.seen == nil {
		m.seen = make(map[string]time.Time)
	}

	// record
	m.seen[id] = now

	// prune
	m.pruneSeenLocked(now)
}

// pruneSeenLocked removes expired entries and enforces SeenMax.
// REQUIRES: m.lock is held.
func (m *meshDaemon) pruneSeenLocked(now time.Time) {
	if m.seen == nil {
		return
	}

	// 1) TTL prune
	cutoff := now.Add(-SeenTTL)
	for k, t := range m.seen {
		if t.Before(cutoff) {
			delete(m.seen, k)
		}
	}

	// 2) Hard cap prune (drop oldest until within limit)
	if len(m.seen) <= SeenMax {
		return
	}

	// Find a threshold by repeatedly deleting the oldest.
	// Simple O(n^2) worst-case, but SeenMax is small and prune is infrequent.
	for len(m.seen) > SeenMax {
		var oldestK string
		var oldestT time.Time
		first := true

		for k, t := range m.seen {
			if first || t.Before(oldestT) {
				oldestK = k
				oldestT = t
				first = false
			}
		}
		if oldestK == "" {
			return
		}
		delete(m.seen, oldestK)
	}
}
