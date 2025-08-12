package util

// IsPowerOfTwo reports whether x is a power of two (> 0).
func IsPowerOfTwo(x uint64) bool {
	return x != 0 && (x&(x-1)) == 0
}

// NextPow2 returns the smallest power of two >= x.
// Special cases:
//   - x == 0  -> 1
//   - if the exact next power would overflow 64 bits, the result is clamped to 1<<63
//
// The implementation uses the classic bit-twiddling "fill" technique.
func NextPow2(x uint64) uint64 {
	if x <= 1 {
		return 1
	}
	x--
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	x++
	// If overflow occurred (wrap to 0), clamp to the highest 64-bit power of two.
	if x == 0 {
		return 1 << 63
	}
	return x
}
