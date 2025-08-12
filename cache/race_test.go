package shardcache

import (
	"context"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A mixed workload of concurrent Set/Get/SetWithTTL/Remove on random keys.
// Should pass under `-race` without detector reports.
func TestRace_Basic(t *testing.T) {
	c := New[string, []byte](Options[string, []byte]{
		Capacity: 8_192,
		Shards:   32,
	})
	t.Cleanup(func() { _ = c.Close() })

	workers := 4 * runtime.GOMAXPROCS(0)
	keyspace := 50_000
	deadline := time.Now().Add(2 * time.Second)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)*9973))
			for time.Now().Before(deadline) {
				k := "k:" + strconv.Itoa(r.Intn(keyspace))
				switch r.Intn(100) {
				case 0, 1, 2, 3, 4: // ~5% — Remove
					c.Remove(k)
				case 5, 6, 7, 8, 9: // ~5% — SetWithTTL
					c.SetWithTTL(k, []byte("x"), time.Duration(10+r.Intn(20))*time.Millisecond)
				case 10, 11, 12, 13, 14, 15, 16, 17, 18, 19: // ~10% — Set
					c.Set(k, []byte("x"))
				default: // ~80% — Get
					c.Get(k)
				}
			}
		}(w)
	}
	wg.Wait()
}

// One hundred goroutines call GetOrLoad on the same key concurrently.
// The Loader should run at most once (singleflight coalescing).
func TestRace_GetOrLoad(t *testing.T) {
	var calls int64

	c := New[string, string](Options[string, string]{
		Capacity: 1024,
		Loader: func(_ context.Context, k string) (string, error) {
			atomic.AddInt64(&calls, 1)
			time.Sleep(2 * time.Millisecond) // simulate I/O
			return "v:" + k, nil
		},
	})
	t.Cleanup(func() { _ = c.Close() })

	const goroutines = 100
	key := "same-key"

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			v, err := c.GetOrLoad(context.Background(), key)
			if err != nil {
				t.Errorf("GetOrLoad error: %v", err)
				return
			}
			if v != "v:"+key {
				t.Errorf("unexpected value: %q", v)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := atomic.LoadInt64(&calls); got > 1 {
		t.Fatalf("loader should run at most once, got %d", got)
	}

	// Subsequent call should be a pure cache hit.
	if v, err := c.GetOrLoad(context.Background(), key); err != nil || v != "v:"+key {
		t.Fatalf("second GetOrLoad failed: v=%q err=%v", v, err)
	}
}
