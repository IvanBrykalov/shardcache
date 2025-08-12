package cache

import (
	"context"
	"time"

	"github.com/IvanBrykalov/lru/policy"
)

// EvictReason explains why an entry was removed.
type EvictReason int

const (
	// EvictPolicy — removed by the active eviction policy (e.g., LRU/2Q/TinyLFU).
	EvictPolicy EvictReason = iota
	// EvictTTL — expired by TTL (lazy eviction on access).
	EvictTTL
	// EvictCapacity — removed to satisfy capacity/cost limits.
	EvictCapacity
)

// Metrics exposes cache-level observability hooks.
// A NoopMetrics implementation is provided and used by default.
type Metrics interface {
	Hit()
	Miss()
	Evict(reason EvictReason)
	Size(entries int, cost int64)
	// Consider adding ObserveLoad(dur) in the future for Loader timing.
}

// Clock provides time in UnixNano; useful for deterministic tests.
type Clock interface{ NowUnixNano() int64 }

// Options configures the cache behavior. Zero values are safe;
// sane defaults are applied in New():
//   - nil Policy   => LRU
//   - Shards <= 0  => auto (rounded up to power of two)
//   - nil Metrics  => NoopMetrics
type Options[K comparable, V any] struct {
	// Capacity is the entry count limit (used together with MaxCost if set).
	Capacity int

	// Shards defines the number of shards. If 0, an automatic value is chosen
	// (≈ 2*GOMAXPROCS) and rounded to the next power of two.
	Shards int

	// Policy is a pluggable eviction policy (LRU/2Q/…); nil => LRU by default.
	Policy policy.Policy[K, V]

	// TTL & SWR
	// DefaultTTL applies to Add/Set when per-key TTL is not provided (0 = no TTL).
	DefaultTTL time.Duration
	// SWR enables serve-stale-while-revalidate windows (reserved for future use).
	SWR time.Duration

	// Cost-based limiting (e.g., bytes). If Cost is non-nil and MaxCost > 0,
	// the cache evicts until both entry count and total cost limits are satisfied.
	Cost    func(v V) int // nil = all entries have equal cost (0)
	MaxCost int64         // total cost limit; 0 disables cost limiting

	// Loader fetches a value on cache miss. Used by GetOrLoad.
	Loader func(ctx context.Context, k K) (V, error)

	// Observability
	// OnEvict is called on eviction under the shard lock; keep callbacks lightweight.
	OnEvict func(k K, v V, reason EvictReason)
	Metrics Metrics

	// Clock allows overriding time source (tests). Nil => time.Now().
	Clock Clock
}
