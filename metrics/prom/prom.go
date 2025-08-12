package prom

import (
	"github.com/IvanBrykalov/lru/cache"
	"github.com/prometheus/client_golang/prometheus"
)

// Adapter implements cache.Metrics and exports Prometheus counters/gauges.
// Safe for concurrent use; all Prometheus metric types are goroutine-safe.
type Adapter struct {
	hits     prometheus.Counter
	misses   prometheus.Counter
	evicts   *prometheus.CounterVec
	sizeEnt  prometheus.Gauge
	sizeCost prometheus.Gauge
}

// New constructs a Prometheus metrics adapter.
//   - reg:          registry to register metrics with (nil => prometheus.DefaultRegisterer)
//   - ns, sub:      Prometheus namespace and subsystem
//   - constLabels:  static labels applied to all metrics (may be nil)
func New(reg prometheus.Registerer, ns, sub string, constLabels prometheus.Labels) *Adapter {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	a := &Adapter{
		hits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "hits_total",
			Help:        "Cache hits",
			ConstLabels: constLabels,
		}),
		misses: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "misses_total",
			Help:        "Cache misses",
			ConstLabels: constLabels,
		}),
		evicts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   ns,
				Subsystem:   sub,
				Name:        "evictions_total",
				Help:        "Cache evictions by reason",
				ConstLabels: constLabels,
			},
			[]string{"reason"},
		),
		sizeEnt: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "size_entries",
			Help:        "Number of resident entries",
			ConstLabels: constLabels,
		}),
		sizeCost: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "size_cost",
			Help:        "Total resident cost",
			ConstLabels: constLabels,
		}),
	}
	reg.MustRegister(a.hits, a.misses, a.evicts, a.sizeEnt, a.sizeCost)
	return a
}

// Hit increments the hit counter.
func (a *Adapter) Hit() { a.hits.Inc() }

// Miss increments the miss counter.
func (a *Adapter) Miss() { a.misses.Inc() }

// Evict increments the eviction counter with a reason label.
func (a *Adapter) Evict(r cache.EvictReason) {
	a.evicts.WithLabelValues(reason(r)).Inc()
}

// Size updates gauges for the number of entries and total cost.
func (a *Adapter) Size(entries int, cost int64) {
	a.sizeEnt.Set(float64(entries))
	a.sizeCost.Set(float64(cost))
}

// reason maps EvictReason to a stable label value.
func reason(r cache.EvictReason) string {
	switch r {
	case cache.EvictTTL:
		return "ttl"
	case cache.EvictCapacity:
		return "capacity"
	default:
		return "policy"
	}
}

// Compile-time check: ensure Adapter implements cache.Metrics.
var _ cache.Metrics = (*Adapter)(nil)
