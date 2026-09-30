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

// queryCache is jobCache for a source whose answer depends on what it was
// asked: one entry per query, each with the same TTL.
//
// Entries are never evicted, only replaced. A process asks as many distinct
// queries as the user has saved searches, which is a handful.
type queryCache[T any] struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[string]*jobCache[T]
}

func newQueryCache[T any](ttl time.Duration) *queryCache[T] {
	return &queryCache[T]{ttl: ttl, entries: make(map[string]*jobCache[T])}
}

// get returns the items cached for query and whether they are still fresh.
func (c *queryCache[T]) get(query string) ([]T, bool) {
	return c.entry(query).get()
}

// put stores a fresh fetch for query.
func (c *queryCache[T]) put(query string, items []T) {
	c.entry(query).put(items)
}

func (c *queryCache[T]) entry(query string) *jobCache[T] {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[query]
	if !ok {
		entry = newJobCache[T](c.ttl)
		c.entries[query] = entry
	}
	return entry
}
