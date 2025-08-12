package policy

// Node is the minimal contract a cache entry must satisfy for a policy.
// It provides read-only access to the key and a pointer to the value.
// The pointer allows in-place updates without re-linking the intrusive node.
type Node[K comparable, V any] interface {
	Key() K
	Value() *V
}

// Hooks expose O(1) list operations that a policy can use to manipulate
// the shard's intrusive MRU/LRU list. Implementations are provided by the shard.
//
// Concurrency: all hook calls happen under the shard lock.
// Important: hooks manage only the list; the shard owns the key->node map.
type Hooks[K comparable, V any] interface {
	// MoveToFront promotes the node to MRU.
	MoveToFront(Node[K, V])
	// PushFront inserts the node at MRU (used on admission).
	PushFront(Node[K, V])
	// Remove detaches the node from the list (map bookkeeping is done by the shard).
	Remove(Node[K, V])
	// Back returns the current LRU node (or nil if empty).
	Back() Node[K, V]
	// Len returns the number of resident nodes in the shard.
	Len() int
}

// ShardPolicy is a per-shard eviction policy instance bound to shard hooks.
// All methods are invoked under the shard lock.
//
// Semantics:
//   - OnAdd may return an eviction candidate (e.g., LRU of a probation queue).
//     The shard will evict that node and subsequently call OnRemove for it.
//   - OnGet/OnUpdate typically promote the node (e.g., move to MRU).
//   - OnRemove is a notification to update policy-internal state
//     (e.g., maintain ghost queues). The shard performs actual deletion.
type ShardPolicy[K comparable, V any] interface {
	OnAdd(Node[K, V]) (evict Node[K, V])
	OnGet(Node[K, V])
	OnUpdate(Node[K, V])
	OnRemove(Node[K, V])
}

// Policy is a factory that creates shard-local policy instances
// bound to a particular shard's hooks.
type Policy[K comparable, V any] interface {
	New(Hooks[K, V]) ShardPolicy[K, V]
}
