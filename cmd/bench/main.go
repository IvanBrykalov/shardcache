// Command bench runs a synthetic workload against the cache and exposes optional pprof/Prometheus endpoints.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on DefaultServeMux
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IvanBrykalov/shardcache/cache"
	pmet "github.com/IvanBrykalov/shardcache/metrics/prom"
	"github.com/IvanBrykalov/shardcache/policy/twoq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// ---- Flags ----
	var (
		capacity = flag.Int("cap", 100_000, "cache capacity (entries)")
		shards   = flag.Int("shards", 0, "number of shards (0=auto)")
		policy   = flag.String("policy", "lru", "eviction policy: lru | 2q")

		workers  = flag.Int("workers", 2*runtime.GOMAXPROCS(0), "number of worker goroutines")
		duration = flag.Duration("duration", 10*time.Second, "benchmark duration")
		readPct  = flag.Int("reads", 80, "read percentage [0..100]")

		keys    = flag.Int("keys", 1_000_000, "keyspace size")
		zipfS   = flag.Float64("zipf_s", 1.1, "Zipf s > 1 (skew)")
		zipfV   = flag.Float64("zipf_v", 1.0, "Zipf v")
		seed    = flag.Int64("seed", time.Now().UnixNano(), "random seed")
		preload = flag.Int("preload", 0, "preload entries (0 = cap/2)")

		pprofAddr   = flag.String("pprof", "", "serve pprof at addr (e.g. :6060); empty = disabled")
		metricsAddr = flag.String("http", ":8080", "serve Prometheus metrics at addr")
	)
	flag.Parse()

	// ---- pprof server (on DefaultServeMux) ----
	if *pprofAddr != "" {
		go func() {
			log.Printf("pprof: serving at %s", *pprofAddr)
			log.Println(http.ListenAndServe(*pprofAddr, nil))
		}()
	}

	// ---- Prometheus metrics (on DefaultServeMux) ----
	metrics := pmet.New(nil, "lru", "bench", nil) // namespace "lru" for consistency
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("metrics: serving at %s", *metricsAddr)
		log.Println(http.ListenAndServe(*metricsAddr, nil))
	}()

	// ---- Build cache ----
	opt := shardcache.Options[string, string]{
		Capacity: *capacity,
		Shards:   *shards,
		Metrics:  metrics,
	}
	switch *policy {
	case "lru":
		// nil => LRU by default
	case "2q":
		// split 2Q queues as a simple default
		opt.Policy = twoq.New[string, string](*capacity/4, *capacity/2)
	default:
		log.Fatalf("unknown policy: %q (use lru or 2q)", *policy)
	}
	c := shardcache.New[string, string](opt)
	defer func() { _ = c.Close() }()

	// ---- Preload half capacity to get a realistic hit-rate ----
	pl := *preload
	if pl == 0 {
		pl = *capacity / 2
	}
	for i := 0; i < pl; i++ {
		k := "k:" + strconv.Itoa(i)
		c.Set(k, "v"+strconv.Itoa(i))
	}

	// ---- Snapshot flags for goroutines ----
	readPctVal := *readPct
	keysMax := uint64(*keys - 1)
	seedBase := *seed
	zipfSVal := *zipfS
	zipfVVal := *zipfV
	workersN := *workers
	if workersN <= 0 {
		workersN = 1
	}

	// ---- Load generation ----
	var reads, writes, hits, misses, total uint64
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(workersN)
	for w := 0; w < workersN; w++ {
		go func(id int) {
			defer wg.Done()

			// Each worker gets its own RNG + Zipf (rand.Rand is NOT goroutine-safe).
			localR := rand.New(rand.NewSource(seedBase + int64(id)*9973))
			localZipf := rand.NewZipf(localR, zipfSVal, zipfVVal, keysMax)

			keyByZipf := func() string {
				return "k:" + strconv.FormatUint(localZipf.Uint64(), 10)
			}

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				atomic.AddUint64(&total, 1)
				if int(localR.Int31n(100)) < readPctVal {
					atomic.AddUint64(&reads, 1)
					if _, ok := c.Get(keyByZipf()); ok {
						atomic.AddUint64(&hits, 1)
					} else {
						atomic.AddUint64(&misses, 1)
					}
				} else {
					atomic.AddUint64(&writes, 1)
					k := keyByZipf()
					c.Set(k, "v"+strconv.Itoa(localR.Int()))
				}
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	// ---- Report ----
	ops := atomic.LoadUint64(&total)
	readsN := atomic.LoadUint64(&reads)
	writesN := atomic.LoadUint64(&writes)
	hitsN := atomic.LoadUint64(&hits)
	missesN := atomic.LoadUint64(&misses)

	hitRate := 0.0
	if readsN > 0 {
		hitRate = float64(hitsN) / float64(readsN) * 100
	}

	fmt.Printf("policy=%s cap=%d shards=%d workers=%d keys=%d dur=%v seed=%d\n",
		*policy, *capacity, *shards, workersN, *keys, elapsed, seedBase)
	fmt.Printf("ops=%d (%.0f ops/s)  reads=%d  writes=%d\n",
		ops, float64(ops)/elapsed.Seconds(), readsN, writesN)
	fmt.Printf("hits=%d  misses=%d  hit-rate=%.2f%%\n", hitsN, missesN, hitRate)
	fmt.Printf("Len()=%d\n", c.Len())
}
