// Package lru implements the LRU eviction policy.
package lru

import "github.com/IvanBrykalov/shardcache/policy"

// lru is a classic "move-to-front" Least-Recently-Used policy.
// It delegates list manipulation to policy.Hooks provided by the shard.
type lru[K comparable, V any] struct {
	h policy.Hooks[K, V]
}

type lruPolicy[K comparable, V any] struct{}

// New returns a Policy factory that constructs per-shard LRU instances.
func New[K comparable, V any]() policy.Policy[K, V] { return lruPolicy[K, V]{} }

// New implements policy.Policy by binding shard hooks and returning
// a shard-local policy instance.
func (lruPolicy[K, V]) New(h policy.Hooks[K, V]) policy.ShardPolicy[K, V] {
	return &lru[K, V]{h: h}
}

// OnAdd places the new entry at MRU. LRU itself doesn't choose evictions;
// the shard enforces capacity/cost limits and performs actual evictions.
func (p *lru[K, V]) OnAdd(n policy.Node[K, V]) (evict policy.Node[K, V]) {
	p.h.PushFront(n)
	return nil
}

// OnGet promotes the entry to MRU.
func (p *lru[K, V]) OnGet(n policy.Node[K, V]) { p.h.MoveToFront(n) }

// OnUpdate promotes the entry to MRU (updates are treated as recent use).
func (p *lru[K, V]) OnUpdate(n policy.Node[K, V]) { p.h.MoveToFront(n) }

// OnRemove is a no-op for pure LRU (nothing to clean up in policy state).
func (p *lru[K, V]) OnRemove(_ policy.Node[K, V]) {}
