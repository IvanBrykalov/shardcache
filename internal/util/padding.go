// Package util contains internal helpers (hashing, sharding, padding).
//revive:disable:var-naming  // allow 'util' as an internal helpers package name
package util

import (
	"sync/atomic"
	"unsafe"
)

// CacheLineSize is a reasonable default for most modern CPUs.
// std has runtime/internal/sys.CacheLineSize but it's unexported.
// 64 works well in practice.
const CacheLineSize = 64

// CacheLinePad is a dummy field used to separate hot fields into distinct
// cache lines and reduce false sharing. Place between groups of hot fields.
type CacheLinePad struct{ _ [CacheLineSize]byte }

// PaddedAtomicInt64 is an atomic int64 padded to exactly one cache line.
// Use when many goroutines update different counters to avoid false sharing.
type PaddedAtomicInt64 struct {
	atomic.Int64
	_ [CacheLineSize - 8]byte // 8 = size of int64; pad to 64 bytes
}

// PaddedAtomicUint64 is the uint64 counterpart padded to one cache line.
type PaddedAtomicUint64 struct {
	atomic.Uint64
	_ [CacheLineSize - 8]byte
}

// PaddedInt64 is a non-atomic variant sized to one cache line.
// Use only when updates happen under a lock.
type PaddedInt64 struct {
	V int64
	_ [CacheLineSize - 8]byte
}

// PaddedUint64 is a non-atomic padded uint64.
type PaddedUint64 struct {
	V uint64
	_ [CacheLineSize - 8]byte
}

// ---- Compile-time size checks (must be exactly one cache line) ----

var (
	_ [CacheLineSize - int(unsafe.Sizeof(PaddedAtomicInt64{}))]byte
	_ [CacheLineSize - int(unsafe.Sizeof(PaddedAtomicUint64{}))]byte
	_ [CacheLineSize - int(unsafe.Sizeof(PaddedInt64{}))]byte
	_ [CacheLineSize - int(unsafe.Sizeof(PaddedUint64{}))]byte
)
