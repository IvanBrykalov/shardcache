//go:build go1.18

package shardcache

import (
	"strings"
	"testing"
)

// Fuzz basic Set/Get/Remove semantics under arbitrary string inputs.
// Guards against panics and ensures core invariants hold.
// NOTE: We cap key/value lengths to avoid pathological memory usage
// during fuzzing (this does not weaken the invariants we check).
func FuzzCache_SetGetRemove(f *testing.F) {
	// Seed corpus: empty, ASCII, Unicode, long strings.
	f.Add("", "")
	f.Add("a", "1")
	f.Add("b", "2")
	f.Add("αβγ", "δ")
	f.Add("emoji🙂", "🙂🙂")
	f.Add("long", strings.Repeat("x", 1024))

	f.Fuzz(func(t *testing.T, k, v string) {
		// Cap lengths to keep memory bounded during fuzzing.
		const limit  = 1 << 12 // 4096
		if len(k) > limit  {
			k = k[:limit ]
		}
		if len(v) > limit  {
			v = v[:limit ]
		}

		c := New[string, string](Options[string, string]{Capacity: 16})
		t.Cleanup(func() { _ = c.Close() })

		// Set -> Get must return the same value.
		c.Set(k, v)
		got, ok := c.Get(k)
		if !ok || got != v {
			t.Fatalf("after Set/Get: want %q, got %q ok=%v", v, got, ok)
		}

		// Add duplicate must not overwrite and must return false.
		if ok := c.Add(k, "other"); ok {
			t.Fatalf("Add duplicate returned true")
		}
		// Value must remain the same after failed Add.
		if got2, ok := c.Get(k); !ok || got2 != v {
			t.Fatalf("after duplicate Add: want %q, got %q ok=%v", v, got2, ok)
		}

		// Remove must delete and return true once.
		if !c.Remove(k) {
			t.Fatalf("Remove must return true")
		}
		if _, ok := c.Get(k); ok {
			t.Fatalf("key must be absent after Remove")
		}

		// After removal, Add should succeed again.
		if ok := c.Add(k, v); !ok {
			t.Fatalf("Add after Remove must return true")
		}
	})
}
