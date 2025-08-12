package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

type fakeClock struct{ t int64 }

func (f *fakeClock) NowUnixNano() int64  { return f.t }
func (f *fakeClock) add(d time.Duration) { f.t += int64(d) }

// Uses a fake clock to avoid timing flakiness.
// Ensures that per-entry TTL is respected.
func TestCache_TTL_FakeClock(t *testing.T) {
	t.Parallel()

	clk := &fakeClock{}
	c := New[string, string](Options[string, string]{Capacity: 4, Clock: clk})
	t.Cleanup(func() { _ = c.Close() })

	c.SetWithTTL("x", "v", 100*time.Millisecond)
	if _, ok := c.Get("x"); !ok {
		t.Fatal("fresh miss")
	}
	clk.add(200 * time.Millisecond)
	if _, ok := c.Get("x"); ok {
		t.Fatal("expired hit")
	}
}

// Basic Add/Set/Get/Remove semantics.
// Add inserts only if key is absent; Set updates; Remove deletes.
func TestCache_BasicAddSetGetRemove(t *testing.T) {
	t.Parallel()

	c := New[string, int](Options[string, int]{Capacity: 8})
	t.Cleanup(func() { _ = c.Close() })

	if !c.Add("a", 1) {
		t.Fatal("Add a=1 must be true")
	}
	if c.Add("a", 2) {
		t.Fatal("Add duplicate must be false")
	}

	c.Set("a", 11)
	if v, ok := c.Get("a"); !ok || v != 11 {
		t.Fatalf("Get a want 11, got %v ok=%v", v, ok)
	}

	if !c.Remove("a") {
		t.Fatal("Remove a must be true")
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("a must be absent after Remove")
	}
}

// Deterministic LRU eviction: single shard, small capacity.
// Accessing "a" promotes it; inserting "c" evicts LRU ("b").
func TestCache_EvictionLRU(t *testing.T) {
	t.Parallel()

	c := New[string, int](Options[string, int]{
		Capacity: 2,
		Shards:   1, // force a single shard so LRU is global
	})
	t.Cleanup(func() { _ = c.Close() })

	c.Set("a", 1) // LRU = a
	c.Set("b", 2) // MRU = b

	if _, ok := c.Get("a"); !ok { // promote a -> MRU
		t.Fatal("expect hit for a")
	}
	c.Set("c", 3) // overflow -> evict LRU (b)

	if _, ok := c.Get("b"); ok {
		t.Fatal("b must be evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a must survive (promoted)")
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatal("c must be present")
	}
}

// Singleflight test: concurrent GetOrLoad calls for the same key
// should trigger the Loader at most once; subsequent calls are cache hits.
func TestCache_GetOrLoad_Singleflight(t *testing.T) {
	var calls int64

	c := New[string, string](Options[string, string]{
		Capacity: 64,
		Loader: func(_ context.Context, k string) (string, error) {
			atomic.AddInt64(&calls, 1)
			time.Sleep(5 * time.Millisecond) // simulate I/O
			return "v:" + k, nil
		},
	})
	t.Cleanup(func() { _ = c.Close() })

	const N = 64
	var g errgroup.Group
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < N; i++ {
		g.Go(func() error {
			v, err := c.GetOrLoad(ctx, "k")
			if err != nil {
				return err
			}
			if v != "v:k" {
				return fmt.Errorf("got %q", v)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("loader must run exactly once, got %d", got)
	}

	if v, err := c.GetOrLoad(context.Background(), "k"); err != nil || v != "v:k" {
		t.Fatalf("second GetOrLoad failed: v=%q err=%v", v, err)
	}
}
