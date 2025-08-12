[![CI](https://github.com/IvanBrykalov/shardcache/actions/workflows/ci.yml/badge.svg)](https://github.com/IvanBrykalov/shardcache/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/IvanBrykalov/shardcache/cache.svg)](https://pkg.go.dev/github.com/IvanBrykalov/shardcache/cache)
[![Release](https://img.shields.io/github/v/release/IvanBrykalov/shardcache?display_name=tag&sort=semver)](https://github.com/IvanBrykalov/shardcache/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## LRU — Sharded in-memory cache for Go (LRU / 2Q)
High-performance in-memory cache for Go featuring:

* Sharding that scales with CPU cores

* LRU by default + pluggable policies (2Q included)

* TTL and GetOrLoad with singleflight de-duplication

* Optional per-entry cost and global MaxCost limit

* Prometheus metrics and pprof profiling

* Clean, generic API (Go 1.20+)

___

## Install
```
go get github.com/IvanBrykalov/shardcache/cache@v0.1.0
```
```
import "github.com/IvanBrykalov/shardcache/cache"
```
___

## Quick start
```
package main

import (
	"fmt"
	"time"

	"github.com/IvanBrykalov/shardcache/cache"
)

func main() {
	c := cache.New[string, string](cache.Options[string, string]{
		Capacity:   1000,
		DefaultTTL: 0, // no default TTL
	})
	defer c.Close()

	c.Set("k", "v")
	if v, ok := c.Get("k"); ok {
		fmt.Println(v) // "v"
	}

	c.SetWithTTL("tmp", "x", 5*time.Second)
	c.Remove("k")
	fmt.Println("size:", c.Len())
}
```
## Fetch on miss (singleflight)
```
c := cache.New[string, string](cache.Options[string, string]{
	Capacity: 1024,
	Loader: func(ctx context.Context, k string) (string, error) {
		// fetch from DB/HTTP/etc
		return "v:" + k, nil
	},
})

v, err := c.GetOrLoad(ctx, "user:42") // concurrent requests are coalesced
```

## Options
```
type Options[K comparable, V any] struct {
	Capacity int                 // entry count limit
	Shards   int                 // 0 = auto (power of two)
	Policy   policy.Policy[K, V] // nil = LRU

	// TTL / SWR
	DefaultTTL time.Duration // 0 = no TTL
	SWR        time.Duration // serve-stale-while-revalidate (optional)

	// Cost limiting
	Cost    func(v V) int // nil = all equal
	MaxCost int64         // total cost limit (>0 enables)

	// Fetch on miss
	Loader func(ctx context.Context, k K) (V, error)

	// Observability
	Metrics cache.Metrics
	OnEvict func(k K, v V, reason cache.EvictReason)

	// Testing clock
	Clock cache.Clock
}
```
## API:
```
Add(k, v) bool            // insert only if absent
Set(k, v)                 // insert or update
SetWithTTL(k, v, ttl)
Get(k) (v, ok bool)
GetOrLoad(ctx, k) (v, error)
Remove(k) bool
Len() int
Close() error
```

## Eviction policies
**LRU is the default**. Policies are pluggable via policy.Policy.Bundled:

* policy/lru — default LRU

* policy/twoq — 2Q (attenuates “one-hit wonders”)

**Use 2Q**:
```
import (
	"github.com/IvanBrykalov/shardcache/cache"
	"github.com/IvanBrykalov/shardcache/policy/twoq"
)

c := cache.New[string, string](cache.Options[string, string]{
	Capacity: 50_000,
	Policy:   twoq.New[string, string](12_500, 25_000), // capIn≈25%, ghosts≈50%
})
```
*The policy interface & hooks (policy.Hooks) are public — you can implement your own (e.g., TinyLFU) without touching the core.*

## Prometheus metrics
Adapter lives in metrics/prom.
```
import (
	"log"
	"net/http"

	"github.com/IvanBrykalov/shardcache/cache"
	pmet "github.com/IvanBrykalov/shardcache/metrics/prom"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	m := pmet.New(nil, "shardcache", "demo", nil)

	c := cache.New[string, string](cache.Options[string, string]{
		Capacity: 10000,
		Metrics:  m,
	})
	defer c.Close()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```
## pprof
Enable in any binary:
```
import _ "net/http/pprof"

go http.ListenAndServe(":6060", nil)
// browse: http://localhost:6060/debug/pprof/
```
CPU profile:
```
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=20
```
## Benchmark tool
cmd/bench generates load, exports Prometheus metrics and pprof:
```
go run ./cmd/bench \
  -cap=100000 -shards=0 -policy=lru \
  -workers=8 -duration=20s -reads=85 \
  -keys=1000000 -zipf_s=1.1 -zipf_v=1.0 \
  -http=:8080 -pprof=:6060
```
Sample output:
```
ops=71.6M (3.58M ops/s)  hits=57.8M  misses=3.0M  hit-rate=95.1%  Len()=100000
```
___
## Design & performance notes
* Sharding uses a power-of-two shard count → fast index with & (n-1) and lower contention.

* Inside a shard: intrusive doubly linked list (MRU↔LRU) + map[K]*node.

* Policies act via hooks; they don’t touch the map/locks → easy to swap.

* Get/Set/Remove are amortized O(1); Len is O(1).
___
## TTL & cost 
* DefaultTTL applies to all Set/Add; SetWithTTL overrides per-item.

* TTL is enforced lazily on read (expired entries are evicted on access).

* With Cost/MaxCost, the cache evicts LRU items until both entry and cost limits are satisfied
__
## Tests
```
go test ./cache -race -v
go test ./cache -bench . -benchmem
```
## Makefile
How to use:

* make test — race-enabled tests across all packages.

* make coverhtml — generates coverage.html you can open in a browser.

* make bench — runs microbenchmarks in ./cache.

* make fuzz — quick fuzz pass for the cache package.

* make lint — runs golangci-lint (auto-installs a pinned version if missing).

* make bench-cmd ARGS="…" — runs your cmd/bench with exactly the flags you pass. For example:
```
make bench-cmd ARGS="-cap=100000 -shards=0 -policy=lru -reads=85 -duration=20s -http=:8080 -pprof=:6060"
```
* make ci — good default for CI pipelines.
## Versioning
We follow SemVer. Before 1.0, minor option tweaks are possible, but the public API principles (sharding, policies, methods) remain stable.
___
## License
MIT © Ivan Brykalov
___
