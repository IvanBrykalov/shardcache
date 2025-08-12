package singleflight

import (
	"context"
	"sync"
)

// Group coalesces concurrent function calls for the same key K so that
// the supplied fn is executed at most once. Other concurrent callers
// wait for the shared result.
//
// Concurrency notes:
//   - The first caller for a given key becomes the leader and runs fn.
//   - Followers wait on c.done. Publishing (val, err) happens-before
//     close(c.done), so reads after <-done observe the final values.
//   - Cancelling ctx in a follower unblocks only that follower; it does
//     NOT cancel the leader's fn. If you need cancellation of the work,
//     pass ctx into fn and handle it there.
type Group[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*call[V]
}

type call[V any] struct {
	done chan struct{} // closed when val/err are published
	val  V
	err  error
}

// Do runs fn once for the given key. Concurrent calls with the same key
// wait for the shared result. If ctx is cancelled in a follower, that
// follower returns ctx.Err() while the leader continues to run fn.
//
// Important:
//   - ctx cancellation does not stop the leader's fn. If cancellation of
//     the underlying work is required, thread ctx into fn and handle it there.
func (g *Group[K, V]) Do(ctx context.Context, key K, fn func() (V, error)) (V, error) {
	// Fast path: an in-flight call exists — wait (respecting ctx).
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[K]*call[V])
	}
	if c, ok := g.m[key]; ok {
		done := c.done
		g.mu.Unlock()

		select {
		case <-done:
			return c.val, c.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}

	// We are the leader for this key.
	c := &call[V]{done: make(chan struct{})}
	g.m[key] = c
	g.mu.Unlock()

	// Execute fn outside the lock.
	v, err := fn()

	// Publish result and wake followers.
	c.val, c.err = v, err
	close(c.done)

	// Remove the in-flight marker.
	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return v, err
}
