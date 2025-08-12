package cache

import (
	"context"
	"math"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/IvanBrykalov/lru/internal/singleflight"
	"github.com/IvanBrykalov/lru/internal/util"
	"github.com/IvanBrykalov/lru/policy/lru"
)

// ErrNoLoader is returned by GetOrLoad when no Loader was configured in Options.
var ErrNoLoader = errorsNew("cache: no Loader provided")

// lightweight local errors.New to avoid importing std 'errors' everywhere
func errorsNew(s string) error { return &strErr{s} }

type strErr struct{ s string }

func (e *strErr) Error() string { return e.s }

// cache is a sharded in-memory KV store with a pluggable eviction policy.
// All methods are safe for concurrent use by multiple goroutines.
type cache[K comparable, V any] struct {
	shards []*shard[K, V]
	hash   func(K) uint64
	closed atomic.Bool

	opt Options[K, V]

	// singleflight group for coalescing concurrent loads in GetOrLoad.
	sf singleflight.Group[K, V]
}

// New constructs a cache with the provided Options.
// Defaults:
//   - nil Metrics  -> NoopMetrics
//   - nil Policy   -> LRU
//   - Shards <= 0  -> auto, rounded up to the next power of two
func New[K comparable, V any](opt Options[K, V]) Cache[K, V] {
	if opt.Capacity <= 0 {
		panic("Capacity must be > 0")
	}
	// default Metrics
	if opt.Metrics == nil {
		opt.Metrics = NoopMetrics{}
	}
	// default Policy: LRU
	if opt.Policy == nil {
		opt.Policy = lru.New[K, V]()
	}

	// number of shards -> power of two
	sh := opt.Shards
	if sh <= 0 {
		auto := 2 * runtime.GOMAXPROCS(0)
		sh = int(util.NextPow2(uint64(auto)))
		if sh < 1 {
			sh = 1
		}
	} else {
		sh = int(util.NextPow2(uint64(sh)))
	}

	cs := make([]*shard[K, V], sh)
	perShardCap := (opt.Capacity + sh - 1) / sh // split capacity evenly (ceil)
	for i := 0; i < sh; i++ {
		cs[i] = newShard[K, V](perShardCap, opt.Policy, opt)
	}

	// return pointer-to-impl as the interface (avoids unexported-return lint)
	return &cache[K, V]{
		shards: cs,
		hash:   util.Fnv64a[K], // fast non-crypto hash for sharding
		opt:    opt,            // keep Options for TTL/Cost/Loader/Metrics
	}
}

// ---- Cache[K,V] implementation ----

// Add inserts k→v only if absent, using DefaultTTL if set.
// Returns false if the key already exists (no update is performed).
func (c *cache[K, V]) Add(k K, v V) bool {
	if c.closed.Load() {
		return false
	}
	s := c.getShard(k)
	ttl := c.defaultDeadline()
	cost := c.costOf(v)
	return s.Add(k, v, ttl, cost)
}

// Set inserts or updates k→v, using DefaultTTL if set,
// and promotes the entry according to the active policy.
func (c *cache[K, V]) Set(k K, v V) {
	if c.closed.Load() {
		return
	}
	s := c.getShard(k)
	ttl := c.defaultDeadline()
	cost := c.costOf(v)
	s.Set(k, v, ttl, cost)
}

// SetWithTTL inserts or updates k→v with a per-key TTL (relative duration).
// A non-positive ttl disables expiration for this entry.
func (c *cache[K, V]) SetWithTTL(k K, v V, ttl time.Duration) {
	if c.closed.Load() {
		return
	}
	s := c.getShard(k)
	cost := c.costOf(v)
	s.Set(k, v, c.deadline(ttl), cost)
}

// Get returns the value for k and a presence flag.
// On hit, the entry is promoted according to the active policy.
func (c *cache[K, V]) Get(k K) (V, bool) {
	if c.closed.Load() {
		var zero V
		return zero, false
	}
	return c.getShard(k).Get(k)
}

// Remove deletes k if present and returns true on success.
func (c *cache[K, V]) Remove(k K) bool {
	if c.closed.Load() {
		return false
	}
	return c.getShard(k).Remove(k)
}

// Len returns the total number of resident entries across all shards.
func (c *cache[K, V]) Len() int {
	total := 0
	for _, s := range c.shards {
		total += s.Len()
	}
	return total
}

// Close marks the cache as closed. Future operations are ignored.
// If background workers are added (TTL/SWR revalidation), they should stop here.
func (c *cache[K, V]) Close() error {
	c.closed.Store(true)
	return nil
}

// GetOrLoad returns the value for k; on miss it loads via Options.Loader,
// coalescing concurrent loads for the same key (singleflight).
// If no Loader is configured, returns ErrNoLoader.
func (c *cache[K, V]) GetOrLoad(ctx context.Context, k K) (V, error) {
	// fast path
	if v, ok := c.Get(k); ok {
		return v, nil
	}
	if c.opt.Loader == nil {
		var zero V
		return zero, ErrNoLoader
	}

	// singleflight: exactly one real load for the key
	return c.sf.Do(ctx, k, func() (V, error) {
		// double-check after flight join
		if v, ok := c.Get(k); ok {
			return v, nil
		}
		v, err := c.opt.Loader(ctx, k)
		if err == nil {
			c.Set(k, v)
		}
		return v, err
	})
}

// ---- helpers ----

// getShard picks a shard by hashing the key and masking with len-1.
// len(c.shards) is guaranteed to be a power of two.
func (c *cache[K, V]) getShard(k K) *shard[K, V] {
	h := c.hash(k)
	idx := int(h) & (len(c.shards) - 1)
	return c.shards[idx]
}

// defaultDeadline returns an absolute deadline based on DefaultTTL.
func (c *cache[K, V]) defaultDeadline() int64 {
	if c.opt.DefaultTTL <= 0 {
		return 0
	}
	return c.deadline(c.opt.DefaultTTL)
}

// deadline converts a relative TTL into an absolute UnixNano deadline.
// A non-positive ttl returns 0 (no expiration).
func (c *cache[K, V]) deadline(ttl time.Duration) int64 {
	if ttl <= 0 {
		return 0
	}
	now := time.Now().UnixNano()
	if c.opt.Clock != nil {
		now = c.opt.Clock.NowUnixNano()
	}
	return now + int64(ttl)
}

// costOf computes the per-entry cost (clamped to int32 range).
func (c *cache[K, V]) costOf(v V) int32 {
	if c.opt.Cost == nil {
		return 0
	}
	iv := c.opt.Cost(v)
	if iv < 0 {
		iv = 0
	}
	// clamp to int32 to avoid overflow
	if iv > math.MaxInt32 {
		iv = math.MaxInt32
	}
	return int32(iv)
}
