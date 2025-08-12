// Package util contains internal helpers (hashing, sharding, padding).
//revive:disable:var-naming  // allow 'util' as an internal helpers package name
package util

import (
	"fmt"
)

// Fnv64a hashes common key types using 64-bit FNV-1a.
// Supported: string, []byte, [16|32|64]byte, all int/uint widths, uintptr, fmt.Stringer.
// For other key types, either convert the key to string or supply a custom hasher upstream.
// Panicking on unsupported types is deliberate to avoid silently poor hashing.
func Fnv64a[K comparable](k K) uint64 {
	switch v := any(k).(type) {
	case string:
		return fnv64aFromBytes([]byte(v))
	case []byte:
		return fnv64aFromBytes(v)
	case [16]byte:
		return fnv64aFromBytes(v[:])
	case [32]byte:
		return fnv64aFromBytes(v[:])
	case [64]byte:
		return fnv64aFromBytes(v[:])

	// Integer-like keys: hash little-endian bytes of the value.
	case uint8:
		return fnv64aFromUint64(uint64(v))
	case uint16:
		return fnv64aFromUint64(uint64(v))
	case uint32:
		return fnv64aFromUint64(uint64(v))
	case uint64:
		return fnv64aFromUint64(v)
	case uint:
		return fnv64aFromUint64(uint64(v))
	case uintptr:
		return fnv64aFromUint64(uint64(v))
	case int8:
		return fnv64aFromUint64(uint64(uint8(v)))
	case int16:
		return fnv64aFromUint64(uint64(uint16(v)))
	case int32:
		return fnv64aFromUint64(uint64(uint32(v)))
	case int64:
		return fnv64aFromUint64(uint64(v))
	case int:
		return fnv64aFromUint64(uint64(v))

	// Fallback for pseudo-keys via String() (avoid if you can).
	case fmt.Stringer:
		return fnv64aFromBytes([]byte(v.String()))
	default:
		panic(fmt.Sprintf("util.Fnv64a: unsupported key type %T; convert key to string or provide a custom hasher", k))
	}
}

const (
	fnvOffset64 = 1469598103934665603
	fnvPrime64  = 1099511628211
)

func fnv64aFromBytes(b []byte) uint64 {
	h := uint64(fnvOffset64)
	for _, c := range b {
		h ^= uint64(c)
		h *= fnvPrime64
	}
	return h
}

func fnv64aFromUint64(u uint64) uint64 {
	// Hash the 8 little-endian bytes of u without allocating.
	h := uint64(fnvOffset64)
	for i := 0; i < 8; i++ {
		h ^= uint64(byte(u))
		h *= fnvPrime64
		u >>= 8
	}
	return h
}
