package util

import "runtime"

// ReasonableShardCount picks a practical default shard count based on CPU
// parallelism. Heuristic: nextPow2(2*GOMAXPROCS), clamped to [1..256].
// This sharply reduces lock contention without bloating memory overhead.
func ReasonableShardCount() int {
	p := runtime.GOMAXPROCS(0)
	if p < 1 {
		p = 1
	}
	// 2×CPU, round up to power of two, then clamp to 256.
	n := int(NextPow2(uint64(p * 2)))
	if n < 1 {
		n = 1
	}
	if n > 256 {
		n = 256
	}
	return n
}

// ShardIndex maps a 64-bit hash to a shard index.
// Assumes shard count is a power of two for the fast mask path,
// but remains correct for arbitrary shard counts (uses modulo).
func ShardIndex(hash uint64, shards int) int {
	if shards <= 1 {
		return 0
	}
	// Fast path if shard count is power of two.
	if IsPowerOfTwo(uint64(shards)) {
		return int(hash & uint64(shards-1))
	}
	return int(hash % uint64(shards))
}
