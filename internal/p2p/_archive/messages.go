package p2p

import (
    "log"
    "sync"
    "time"
)

// Message is a simple payload routed over the mesh.
type Message struct {
    From      string `json:"from"`
    To        string `json:"to"`
    Body      string `json:"body"`
    Timestamp int64  `json:"timestamp"`
    Hops      int    `json:"hops"`
}

var (
    inboxMu sync.RWMutex
    inbox   []Message
)

// storeLocalMessage appends a message to this node's local inbox.
func storeLocalMessage(from, to, body string, hops int) {
    inboxMu.Lock()
    defer inboxMu.Unlock()

    msg := Message{
        From:      from,
        To:        to,
        Body:      body,
        Timestamp: time.Now().Unix(),
        Hops:      hops,
    }
    inbox = append(inbox, msg)
    log.Printf("✉️ inbox: %+v", msg)
}

// inboxSnapshot returns a copy of the local inbox.
func inboxSnapshot() []Message {
    inboxMu.RLock()
    defer inboxMu.RUnlock()

    out := make([]Message, len(inbox))
    copy(out, inbox)
    return out
}
