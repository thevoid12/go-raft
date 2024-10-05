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

// Append adds a new entry to the log
func (l *Log) Append(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

// GetEntries returns all log entries
func (l *Log) GetEntries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.entries
}

// GetLastIndex returns the index of the last log entry
func (l *Log) GetLastIndex() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// GetLastTerm returns the term of the last log entry
func (l *Log) GetLastTerm() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		return 0
	}
	return l.entries[len(l.entries)-1].Term
}

// GetEntry returns the log entry at a specific index
func (l *Log) GetEntry(index int) (LogEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < 0 || index >= len(l.entries) {
		return LogEntry{}, false
	}
	return l.entries[index], true
}

// Truncate truncates the log to the specified index (exclusive)
func (l *Log) Truncate(index int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < len(l.entries) {
		l.entries = l.entries[:index]
	}
}
