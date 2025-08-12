// Package shardcache provides a fast, generic, sharded in-memory cache with
// pluggable eviction policies (LRU by default), per-entry TTL, optional
// singleflight loading, lightweight metrics hooks, and cost-based capacity.
//
// Design
//
//   - Concurrency: the cache is split into shards, each protected by an
//     RWMutex. The default shard count is chosen by a heuristic
//     (ReasonableShardCount) and is a power of two. Picking shards reduces
//     contention while keeping memory overhead small.
//
//   - Storage: each shard keeps a map[K]*node for lookups and an intrusive
//     MRU↔LRU doubly linked list for ordering. All operations are O(1) expected.
//
//   - Policies: eviction policy is pluggable via the policy package.
//     LRU is the default. A 2Q policy is provided (resists scan pollution).
//     More policies (e.g. WTinyLFU) can be added without changing the shard.
//
//   - TTL: entries can have per-item deadlines (UnixNano). Expiration is lazy
//     on read (and also enforced while the shard trims to capacity).
//
//   - Cost/MaxCost: besides entry count (Capacity), you may account a user-defined
//     "cost" per value (Options.Cost) and enforce a global MaxCost. Shards split
//     the MaxCost budget evenly.
//
//   - GetOrLoad: coalesces concurrent loads for the same key using singleflight.
//     If Loader is nil, GetOrLoad returns ErrNoLoader.
//
//   - Metrics: Options.Metrics receives Hit/Miss/Evict/Size signals.
//     By default NoopMetrics is used; plug a Prometheus adapter to export metrics.
//
//   - Callbacks: Options.OnEvict(k, v, reason) is called for every eviction
//     (reason is one of EvictPolicy, EvictTTL, EvictCapacity).
//
// Basic usage
//
//	// Create an LRU cache with capacity for 10k entries.
//	c := cache.New[string, []byte](cache.Options[string, []byte]{Capacity: 10_000})
//	c.Set("a", []byte("1"))
//	if v, ok := c.Get("a"); ok {
//	    _ = v // use value
//	}
//	c.Remove("a")
//
// With TTL
//
//	c := cache.New[string, string](cache.Options[string, string]{Capacity: 1024})
//	c.SetWithTTL("tmp", "v", 200*time.Millisecond)
//	time.Sleep(300*time.Millisecond)
//	_, ok := c.Get("tmp") // ok == false (expired)
//
// With GetOrLoad (singleflight)
//
//	c := cache.New[string, string](cache.Options[string, string]{
//	    Capacity: 1024,
//	    Loader: func(ctx context.Context, k string) (string, error) {
//	        // e.g. fetch from DB
//	        return "v:" + k, nil
//	    },
//	})
//	v, err := c.GetOrLoad(context.Background(), "key")
//
// Using an alternative policy (2Q)
//
//	c := cache.New[string, string](cache.Options[string, string]{
//	    Capacity: 50_000,
//	    Policy:   twoq.New[string, string](12_500 /* A1in ≈ 25% */, 25_000 /* ghosts */),
//	})
//
// Exporting metrics (example Prometheus adapter)
//
//	m := prom.New(nil, "cachex", "demo") // implements Metrics
//	c := cache.New[string, []byte](cache.Options[string, []byte]{
//	    Capacity: 10_000,
//	    Metrics:  m,
//	})
//
// Thread-safety & complexity
//
// All methods on Cache are safe for concurrent use. Typical operation cost is
// O(1) expected time: one map access and a constant amount of pointer fixes.
// Eviction work is also O(1) per removed item.
//
// See package cache/options.go for all available Options fields and package
// policy for the Policy/Hooks interfaces used to implement custom strategies.
package shardcache
