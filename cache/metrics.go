package cache

// NoopMetrics is a drop-in Metrics implementation that does nothing.
// It is safe for concurrent use and intended as the default when
// no observability backend is configured.
type NoopMetrics struct{}

func (NoopMetrics) Hit()                         {}
func (NoopMetrics) Miss()                        {}
func (NoopMetrics) Evict(EvictReason)            {}
func (NoopMetrics) Size(entries int, cost int64) {}

// Ensure NoopMetrics implements the Metrics interface at compile time.
var _ Metrics = NoopMetrics{}
