// Command basic using the cache.
package main

import (
	"fmt"
	"time"

	"github.com/IvanBrykalov/shardcache/cache"
)

func main() {
	// By default the eviction policy is LRU.
	c := shardcache.New[string, string](shardcache.Options[string, string]{
		Capacity:   5, // entry count limit
		DefaultTTL: 0, // no default TTL
	})
	defer func() { _ = c.Close() }()

	// Add: insert only if key is absent (no update). Returns false on duplicate.
	fmt.Println("Add a=1  ->", c.Add("a", "1")) // true
	fmt.Println("Add a=2  ->", c.Add("a", "2")) // false (duplicate)

	// Set: insert or update (promotes according to the policy).
	c.Set("b", "2")
	c.Set("a", "1*") // update existing value

	// Get: returns value and promotes entry to MRU (for LRU policy).
	if v, ok := c.Get("a"); ok {
		fmt.Println("Get a   ->", v)
	}
	if _, ok := c.Get("zzz"); !ok {
		fmt.Println("Get zzz -> miss")
	}

	// Remove: deletes the key if present.
	fmt.Println("Remove b ->", c.Remove("b"))

	// Per-key TTL: SetWithTTL overrides DefaultTTL for this entry only.
	c.SetWithTTL("tmp", "ephemeral", 200*time.Millisecond)
	time.Sleep(120 * time.Millisecond)
	fmt.Println("Get tmp (fresh):", must(c.Get("tmp")))
	time.Sleep(150 * time.Millisecond)
	if _, ok := c.Get("tmp"); !ok {
		fmt.Println("Get tmp (expired): miss")
	}

	fmt.Println("Len() ->", c.Len())
}

// must unwraps a (value, ok) pair and panics on a miss.
// Handy for concise example code.
func must[V any](v V, ok bool) V {
	if !ok {
		panic("unexpected miss")
	}
	return v
}
