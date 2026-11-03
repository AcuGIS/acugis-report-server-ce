package audit

import (
	"context"
	"sync"
	"time"
)

// MemoryWriter is an in-memory EventWriter for tests.
type MemoryWriter struct {
	mu     sync.Mutex
	Events []Event
}

// Configured always returns true.
func (m *MemoryWriter) Configured() bool { return m != nil }

// Insert appends an event.
func (m *MemoryWriter) Insert(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, e)
	return nil
}

// Reset clears stored events.
func (m *MemoryWriter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = nil
}

// Snapshot returns a copy of recorded events.
func (m *MemoryWriter) Snapshot() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Event, len(m.Events))
	copy(out, m.Events)
	return out
}

// MemoryStore is an in-memory audit store supporting Insert and retention deletes for tests.
type MemoryStore struct {
	MemoryWriter
}

// DeleteOlderThan removes events with CreatedAt before cutoff.
func (m *MemoryStore) DeleteOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []Event
	var deleted int64
	for _, e := range m.Events {
		if e.CreatedAt.Before(cutoff) {
			deleted++
			continue
		}
		kept = append(kept, e)
	}
	m.Events = kept
	return deleted, nil
}
