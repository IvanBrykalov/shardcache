// Command shards demonstrates sharded cache usage with the 2Q policy.
package main

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/IvanBrykalov/shardcache/cache"
	"github.com/IvanBrykalov/shardcache/policy/twoq"
)

func main() {
	// Tunables that usually depend on the environment.
	capacity := 50_000
	shards := 64 // power of two recommended
	workers := 8 * runtime.GOMAXPROCS(0)
	keys := 200_000

	// IMPORTANT: For 2Q on a sharded cache, size the queues per shard.
	perShardCap := (capacity + shards - 1) / shards
	a1in := perShardCap / 4  // ~25% of shard capacity
	ghost := perShardCap / 2 // ~50% of shard capacity

	c := shardcache.New[string, string](shardcache.Options[string, string]{
		Capacity: capacity,
		Shards:   shards,
		Policy:   twoq.New[string, string](a1in, ghost), // per-shard 2Q sizing
	})
	defer func() { _ = c.Close() }()

	var wg sync.WaitGroup
	start := time.Now()

	// Populate concurrently: each worker walks a disjoint stride of keys.
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := id; i < keys; i += workers {
				k := "k:" + strconv.Itoa(i)
				c.Set(k, "v"+strconv.Itoa(i))
				// Occasional Get to promote a subset into MRU.
				if i%3 == 0 {
					c.Get(k)
				}
			}
		}(w)
	}
	wg.Wait()

	// A small round of repeated reads to exercise hit-path.
	for i := 0; i < 10_000; i++ {
		k := "k:" + strconv.Itoa(i)
		c.Get(k)
	}

	fmt.Printf("done in %v, Len=%d (cap=%d, shards=%d, GOMAXPROCS=%d)\n",
		time.Since(start), c.Len(), capacity, shards, runtime.GOMAXPROCS(0))
}
