package cache

// NoopMetrics is a Metrics implementation that does nothing.
type NoopMetrics struct{}

// Hit records a cache hit. NoopMetrics ignores the call.
func (NoopMetrics) Hit() {}

// Miss records a cache miss. NoopMetrics ignores the call.
func (NoopMetrics) Miss() {}

// Evict records an eviction reason. NoopMetrics ignores the call.
func (NoopMetrics) Evict(EvictReason) {}

// Size reports current resident size and cost. NoopMetrics ignores the call.
func (NoopMetrics) Size(_ int, _ int64) {}
