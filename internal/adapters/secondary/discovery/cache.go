package discovery

import (
	"sync"
	"time"
)

// jobCache holds a source's last fetch for a while, so repeated searches
// do not poll an API faster than its data actually changes.
//
// Each source states its own TTL from what the source says about itself.
// Arbeitnow and Remotive predate this type and keep their own copies of
// the same logic; they are left alone rather than refactored alongside an
// unrelated change.
type jobCache[T any] struct {
	ttl time.Duration

	mu        sync.Mutex
	fetchedAt time.Time
	items     []T
}

func newJobCache[T any](ttl time.Duration) *jobCache[T] {
	return &jobCache[T]{ttl: ttl}
}

// get returns the cached items and whether they are still fresh.
func (c *jobCache[T]) get() ([]T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil || time.Since(c.fetchedAt) >= c.ttl {
		return nil, false
	}
	return c.items, true
}

// put stores a fresh fetch.
func (c *jobCache[T]) put(items []T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = items
	c.fetchedAt = time.Now()
}
