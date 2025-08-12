package cache

import (
	"context"
	"time"
)

// Cache is a sharded, in-memory key/value cache interface.
// All methods are safe for concurrent use by multiple goroutines.
//
// Typical complexity for operations is amortized O(1):
// a map lookup plus constant-time list adjustments under a shard lock.
type Cache[K comparable, V any] interface {
	// Add inserts k→v only if k is not present.
	// It uses the cache's DefaultTTL (if any).
	// Returns false if the key already exists (no update is performed).
	Add(k K, v V) bool

	// Set inserts or updates k→v.
	// It uses the cache's DefaultTTL (if any), and promotes the entry
	// according to the active eviction policy (e.g., LRU).
	Set(k K, v V)

	// Get returns the value for k and a boolean flag indicating presence.
	// On hit, the entry is promoted according to the policy.
	Get(k K) (V, bool)

	// Remove deletes k if present and returns true on success.
	Remove(k K) bool

	// Len returns the total number of resident entries across all shards.
	Len() int

	// Close stops background workers (if any) and marks the cache closed.
	// Current implementation is a soft close and returns nil.
	Close() error

	// SetWithTTL inserts or updates k→v with a per-key TTL (relative duration).
	// A non-positive ttl disables expiration for this entry.
	SetWithTTL(k K, v V, ttl time.Duration)

	// GetOrLoad returns the value for k, loading it via Options.Loader on miss.
	// Concurrent loads for the same key are coalesced (singleflight).
	// If no Loader was configured, returns ErrNoLoader.
	GetOrLoad(ctx context.Context, k K) (V, error)
}
