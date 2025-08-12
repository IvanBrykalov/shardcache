package main

import (
	"log"
	"net/http"

	"github.com/IvanBrykalov/lru/cache"
	"github.com/IvanBrykalov/lru/metrics/prom"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Register Prometheus metrics on the default registry.
	// You can pass const labels instead of nil, e.g.:
	// prom.New(nil, "lru", "demo", prometheus.Labels{"app": "example"})
	m := prom.New(nil, "lru", "demo", nil)

	// Build a small cache and wire metrics.
	c := cache.New[string, []byte](cache.Options[string, []byte]{
		Capacity: 10000,
		Metrics:  m,
	})
	defer c.Close()

	// Generate a tiny bit of traffic so counters are non-zero.
	c.Set("a", []byte("1"))
	c.Get("a") // hit
	c.Get("b") // miss

	// Expose /metrics for Prometheus scraping.
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}
