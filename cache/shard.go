package cache

import (
	"sync"
	"time"

	"github.com/IvanBrykalov/lru/internal/util"
	"github.com/IvanBrykalov/lru/policy"
)

// shard is an independent partition of the cache with its own lock, map,
// and an intrusive doubly linked list (head=MRU, tail=LRU).
type shard[K comparable, V any] struct {
	// ---- guarded by mu ----
	mu      sync.RWMutex
	m       map[K]*node[K, V]
	head    *node[K, V] // MRU
	tail    *node[K, V] // LRU
	len     int         // number of resident entries
	cost    int64       // total cost (if MaxCost is enabled)
	cap     int         // per-shard entry capacity
	maxCost int64       // per-shard cost limit (0 = disabled)

	// Policy and options (policy uses hooks to manipulate the list).
	pol policy.ShardPolicy[K, V]
	opt Options[K, V]

	// ---- hot counters (separate cache lines to avoid false sharing) ----
	_      util.CacheLinePad
	hits   util.PaddedAtomicInt64
	misses util.PaddedAtomicInt64
	evicts util.PaddedAtomicUint64
}

// newShard initializes a shard with per-shard capacity, policy factory, and options.
// maxCost is derived by splitting opt.MaxCost evenly across shards.
func newShard[K comparable, V any](capacity int, pol policy.Policy[K, V], opt Options[K, V]) *shard[K, V] {
	s := &shard[K, V]{
		m:   make(map[K]*node[K, V], capacity),
		cap: capacity,
		opt: opt,
	}

	// Split global MaxCost across shards (ceil division).
	if opt.MaxCost > 0 {
		shards := opt.Shards
		if shards <= 0 {
			shards = util.ReasonableShardCount()
		}
		s.maxCost = (opt.MaxCost + int64(shards) - 1) / int64(shards)
	}

	// Wrap this shard with policy hooks.
	h := shardHooks[K, V]{s: s}
	s.pol = pol.New(h)
	return s
}

// Add inserts a NEW entry (no update) as MRU via policy hooks.
// ttl is an absolute UnixNano deadline (0 = no TTL); cost is the logical weight (0 = equal).
// Returns false if the key already exists.
func (s *shard[K, V]) Add(k K, v V, ttl int64, cost int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.m[k]; exists {
		return false
	}
	n := &node[K, V]{key: k, val: v, exp: ttl, cost: cost}
	s.m[k] = n

	// Let the policy place/promote (and optionally suggest an eviction).
	if ev := s.pol.OnAdd(n); ev != nil {
		s.evictNode(ev.(*node[K, V]), EvictPolicy)
	}

	// Enforce per-shard limits after insertion.
	s.enforceLimitsLocked()
	return true
}

// Set inserts or updates an entry and promotes it according to the policy.
func (s *shard[K, V]) Set(k K, v V, ttl int64, cost int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n, ok := s.m[k]; ok {
		// In-place update: adjust cost delta and promote.
		oldCost := int64(n.cost)
		n.val = v
		n.exp = ttl
		n.cost = cost
		s.cost += int64(cost) - oldCost

		s.pol.OnUpdate(n)
		s.enforceLimitsLocked()
		return
	}

	// New entry path.
	n := &node[K, V]{key: k, val: v, exp: ttl, cost: cost}
	s.m[k] = n

	if ev := s.pol.OnAdd(n); ev != nil {
		s.evictNode(ev.(*node[K, V]), EvictPolicy)
	}
	s.enforceLimitsLocked()
}

// Get returns the value and promotes the entry according to the policy.
// TTL: if expired, the entry is evicted and a miss is returned.
func (s *shard[K, V]) Get(k K) (V, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.m[k]
	if !ok {
		s.misses.Add(1)
		s.opt.Metrics.Miss()
		var zero V
		return zero, false
	}
	if s.expiredLocked(n) {
		s.evictNode(n, EvictTTL)
		s.misses.Add(1)
		s.opt.Metrics.Miss()
		var zero V
		return zero, false
	}

	s.pol.OnGet(n)
	s.hits.Add(1)
	s.opt.Metrics.Hit()
	return n.val, true
}

// Remove deletes an entry by key. Returns true if the entry existed.
func (s *shard[K, V]) Remove(k K) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.m[k]
	if !ok {
		return false
	}
	s.pol.OnRemove(n)
	s.removeNode(n)
	delete(s.m, k)
	// Note: explicit Remove is not counted as an eviction in metrics;
	// add a dedicated "deletes" counter if needed.
	return true
}

// Len returns the number of resident entries in this shard.
func (s *shard[K, V]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.len
}

// -------------------- internals (mu held) --------------------

func (s *shard[K, V]) expiredLocked(n *node[K, V]) bool {
	if n.exp == 0 {
		return false
	}
	return s.now() > n.exp
}

func (s *shard[K, V]) now() int64 {
	if s.opt.Clock != nil {
		return s.opt.Clock.NowUnixNano()
	}
	return time.Now().UnixNano()
}

// insertFront inserts n at MRU in O(1).
func (s *shard[K, V]) insertFront(n *node[K, V]) {
	n.prev = nil
	n.next = s.head
	if s.head != nil {
		s.head.prev = n
	}
	s.head = n
	if s.tail == nil {
		s.tail = n
	}
	s.len++
	s.cost += int64(n.cost)
}

// moveToFront promotes n to MRU in O(1).
func (s *shard[K, V]) moveToFront(n *node[K, V]) {
	if n == s.head {
		return
	}
	// detach
	if n.prev != nil {
		n.prev.next = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	}
	if s.tail == n {
		s.tail = n.prev
	}
	// insert at head
	n.prev = nil
	n.next = s.head
	if s.head != nil {
		s.head.prev = n
	}
	s.head = n
	if s.tail == nil {
		s.tail = n
	}
}

// removeNode removes n from the list and updates counters in O(1).
func (s *shard[K, V]) removeNode(n *node[K, V]) {
	if n.prev != nil {
		n.prev.next = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	}
	if s.head == n {
		s.head = n.next
	}
	if s.tail == n {
		s.tail = n.prev
	}
	n.prev, n.next = nil, nil
	s.len--
	s.cost -= int64(n.cost)
	if s.cost < 0 {
		s.cost = 0
	}
}

// back returns the current LRU node in O(1).
func (s *shard[K, V]) back() *node[K, V] { return s.tail }

// evictNode removes the node, updates metrics/counters, and calls OnEvict.
func (s *shard[K, V]) evictNode(n *node[K, V], reason EvictReason) {
	s.pol.OnRemove(n)
	s.removeNode(n)
	delete(s.m, n.key)
	s.evicts.Add(1)
	s.opt.Metrics.Evict(reason)
	if cb := s.opt.OnEvict; cb != nil {
		// Note: calling callbacks under the lock is safer but may add latency.
		// If you move this outside the lock later, pass copies of key/value.
		cb(n.key, n.val, reason)
	}
}

// enforceLimitsLocked evicts LRU items until both count and cost limits are satisfied.
func (s *shard[K, V]) enforceLimitsLocked() {
	// Count limit
	for s.len > s.cap {
		if tail := s.back(); tail != nil {
			s.evictNode(tail, EvictPolicy)
		} else {
			break
		}
	}
	// Cost limit
	if s.maxCost > 0 {
		for s.cost > s.maxCost {
			if tail := s.back(); tail != nil {
				s.evictNode(tail, EvictCapacity)
			} else {
				break
			}
		}
	}
	s.opt.Metrics.Size(s.len, s.cost)
}

// -------------------- policy hooks --------------------

// shardHooks adapts the shard's list operations to policy.Hooks.
type shardHooks[K comparable, V any] struct{ s *shard[K, V] }

func (h shardHooks[K, V]) MoveToFront(x policy.Node[K, V]) { h.s.moveToFront(x.(*node[K, V])) }
func (h shardHooks[K, V]) PushFront(x policy.Node[K, V])   { h.s.insertFront(x.(*node[K, V])) }
func (h shardHooks[K, V]) Remove(x policy.Node[K, V]) {
	// Policies call Remove while the shard lock is held.
	// Map bookkeeping is performed by the shard itself.
	h.s.removeNode(x.(*node[K, V]))
}
func (h shardHooks[K, V]) Back() policy.Node[K, V] { return h.s.back() }
func (h shardHooks[K, V]) Len() int                { return h.s.len }
