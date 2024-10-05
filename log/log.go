// this package log helps in Managing log operations and replication.
package log

import (
	"sync"
)

type Log struct {
	mu      sync.Mutex
	entries []LogEntry
}

func NewLog() *Log {
	return &Log{
		entries: make([]LogEntry, 0),
	}
}

func (l *Log) Append(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *Log) GetEntries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.entries
}
