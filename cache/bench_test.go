package cache

import (
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"
)

// benchmarkMix exercises a read/write mix against a warm cache.
// It uses parallel workers (RunParallel spawns GOMAXPROCS goroutines).
// String keys include strconv/concat costs and often allocate, which is fine
// for an end-to-end benchmark.
func benchmarkMix(b *testing.B, readsPct int) {
	c := New[string, string](Options[string, string]{
		Capacity: 100_000,
	})
	b.Cleanup(func() { _ = c.Close() })

	// Preload half the capacity to get a realistic hit-rate.
	for i := 0; i < 50_000; i++ {
		k := "k:" + strconv.Itoa(i)
		c.Set(k, "v")
	}

	// Report per-op allocations for a rough idea where costs go.
	b.ReportAllocs()
	b.ResetTimer()

	var seed int64 = 1
	keyMask := (1 << 16) - 1 // hot keyspace (power of two for fast &-mask)

	b.RunParallel(func(pb *testing.PB) {
		// Independent RNG stream for each worker.
		r := rand.New(rand.NewSource(atomic.AddInt64(&seed, 1)))
		i := 0
		for pb.Next() {
			k := "k:" + strconv.Itoa(i&keyMask)
			if r.Intn(100) < readsPct {
				c.Get(k)
			} else {
				c.Set(k, "v")
			}
			i++
		}
	})
}

func BenchmarkCache_90r10w(b *testing.B) { benchmarkMix(b, 90) }
func BenchmarkCache_50r50w(b *testing.B) { benchmarkMix(b, 50) }

// benchmarkMixInt is the same workload but with int keys.
// This removes strconv/alloc noise and better exposes the cache hot path.
func benchmarkMixInt(b *testing.B, readsPct int) {
	c := New[int, int](Options[int, int]{
		Capacity: 100_000,
	})
	b.Cleanup(func() { _ = c.Close() })

	for i := 0; i < 50_000; i++ {
		c.Set(i, 1)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var seed int64 = 1
	keyMask := (1 << 16) - 1

	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(atomic.AddInt64(&seed, 1)))
		i := 0
		for pb.Next() {
			k := i & keyMask
			if r.Intn(100) < readsPct {
				c.Get(k)
			} else {
				c.Set(k, 1)
			}
			i++
		}
	})
}

func BenchmarkCache_IntKeys_90r10w(b *testing.B) { benchmarkMixInt(b, 90) }
func BenchmarkCache_IntKeys_50r50w(b *testing.B) { benchmarkMixInt(b, 50) }
