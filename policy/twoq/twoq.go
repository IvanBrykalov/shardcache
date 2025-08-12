// policy/twoq/twoq.go
package twoq

import (
	"container/list"

	"github.com/IvanBrykalov/lru/policy"
)

// twoQ implements the 2Q eviction policy.
//
// Resident queues:
//   • A1in (younger queue) — its own list + index by Node; admits first-time entries
//   • Am   (mature queue)  — nodes not present in inIdx; ordering is driven by shard hooks
//
// Ghost A1out: keys only (no values), tracks recently evicted A1in keys to give them
// a second chance (bypass A1in on re-admission).
//
// Concurrency: all methods are called under the shard lock.
type twoQ[K comparable, V any] struct {
	h policy.Hooks[K, V]

	capIn    int // A1in capacity (per-shard)
	capGhost int // A1out (ghost) capacity (per-shard)

	// A1in: MRU at Front() -> LRU at Back()
	inList *list.List
	// Fast membership check for "is node in A1in?"
	inIdx map[policy.Node[K, V]]*list.Element // element.Value is policy.Node[K,V]

	// A1out (ghosts): keys only, MRU at Front() -> LRU at Back()
	ghostList *list.List
	ghostIdx  map[K]*list.Element // key -> element in ghostList (element.Value is K)
}

// New constructs a 2Q policy factory.
// Common choices: capIn ≈ 25% of shard capacity; capGhost ≈ 50–100% of shard capacity.
// NOTE: When used with a sharded cache, pass *per-shard* sizes here.
func New[K comparable, V any](capIn, capGhost int) policy.Policy[K, V] {
	if capIn < 1 {
		capIn = 1
	}
	if capGhost < 1 {
		capGhost = 1
	}
	return twoQPolicy[K, V]{capIn: capIn, capGhost: capGhost}
}

type twoQPolicy[K comparable, V any] struct {
	capIn    int
	capGhost int
}

func (p twoQPolicy[K, V]) New(h policy.Hooks[K, V]) policy.ShardPolicy[K, V] {
	return &twoQ[K, V]{
		h:         h,
		capIn:     p.capIn,
		capGhost:  p.capGhost,
		inList:    list.New(),
		inIdx:     make(map[policy.Node[K, V]]*list.Element),
		ghostList: list.New(),
		ghostIdx:  make(map[K]*list.Element),
	}
}

// OnAdd admission rules:
//   • If key is present in ghosts (A1out), bypass A1in and admit directly to Am (MRU).
//     Also remove the ghost entry.
//   • Otherwise admit into A1in (and MRU in the shard list via hooks).
//   • If A1in overflows, return its LRU candidate to the shard for eviction.
func (q *twoQ[K, V]) OnAdd(n policy.Node[K, V]) (evict policy.Node[K, V]) {
	k := n.Key()
	if ge, ok := q.ghostIdx[k]; ok {
		// Second chance: promote from ghosts directly into Am (skip A1in).
		q.ghostList.Remove(ge)
		delete(q.ghostIdx, k)
		q.h.PushFront(n) // MRU in shard list (Am)
		return nil
	}

	// First-time admission: insert into A1in and MRU of the shard list.
	q.h.PushFront(n)
	q.inIdx[n] = q.inList.PushFront(n)

	// If A1in is over capacity, propose its LRU for eviction.
	if q.inList.Len() > q.capIn {
		if lruEl := q.inList.Back(); lruEl != nil {
			return lruEl.Value.(policy.Node[K, V])
		}
	}
	return nil
}

// OnGet: if the node was in A1in, remove it from A1in (promotion to Am),
// then move it to MRU in the shard list.
func (q *twoQ[K, V]) OnGet(n policy.Node[K, V]) {
	if el, ok := q.inIdx[n]; ok {
		q.inList.Remove(el)
		delete(q.inIdx, n)
	}
	q.h.MoveToFront(n)
}

// OnUpdate follows OnGet semantics (updates count as recent use).
func (q *twoQ[K, V]) OnUpdate(n policy.Node[K, V]) { q.OnGet(n) }

// OnRemove:
//   • If the node was in A1in, add its key to ghosts (A1out), respecting capGhost.
//   • Removals from Am do NOT populate ghosts.
func (q *twoQ[K, V]) OnRemove(n policy.Node[K, V]) {
	if el, ok := q.inIdx[n]; ok {
		// Remove from A1in tracking.
		q.inList.Remove(el)
		delete(q.inIdx, n)

		k := n.Key()

		// Insert/move ghost to MRU.
		if old := q.ghostIdx[k]; old != nil {
			q.ghostList.Remove(old)
		}
		q.ghostIdx[k] = q.ghostList.PushFront(k)

		// Enforce ghost capacity (drop LRU ghosts).
		for q.ghostList.Len() > q.capGhost {
			tail := q.ghostList.Back()
			if tail == nil {
				break
			}
			kk := tail.Value.(K)
			delete(q.ghostIdx, kk)
			q.ghostList.Remove(tail)
		}
	}
}
