# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- …

### Changed
- …

### Fixed
- …

---

## [v0.1.0] - 2025-08-12
### Added
- **Sharded in-memory cache** with lock-per-shard design (amortized O(1) ops).
- **Default LRU policy** (`policy/lru`) and **2Q policy** (`policy/twoq`).
- **Per-entry TTL** (`SetWithTTL`) and **default TTL** via options.
- **Max cost** limiting with user-defined `Cost(v)` and `MaxCost`.
- **Singleflight loader**: `GetOrLoad(ctx, k)` collapses concurrent loads per key.
- **Prometheus metrics adapter** (`metrics/prom`): hits, misses, evictions by reason, size gauges.
- **Race-safe** implementation with tests (`-race` clean).
- **Benchmark tool** (`cmd/bench`) with Zipf key distribution, metrics & pprof endpoints.
- **Examples**: basic usage, shards demo, HTTP metrics demo.
- **CI** (GitHub Actions) for vet, tests with race detector, and lint.

### Changed
- N/A (initial release)

### Fixed
- N/A (initial release)

### Install
```bash
go get github.com/IvanBrykalov/lru@v0.1.0
```

## Compatibility
* Go 1.24+.