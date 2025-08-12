package cache

// node is an intrusive doubly linked list element owned by a shard.
// It stores the key/value alongside list links and metadata used by
// eviction policies and TTL/cost accounting.
type node[K comparable, V any] struct {
	key K
	val V

	// Intrusive list links: head is MRU, tail is LRU.
	prev *node[K, V]
	next *node[K, V]

	// Absolute expiration deadline in UnixNano.
	// Zero means "no TTL".
	exp int64

	// Logical "cost" used when MaxCost is enabled.
	// Entries are evicted until both length and cost limits are satisfied.
	cost int32

	// Reserved for policy-specific metadata (e.g., class/segment for 2Q/TinyLFU).
	// Add fields here when a policy needs to tag nodes without map lookups.
	// e.g. class uint8
}

// Key returns the node key (part of policy.Node interface).
func (n *node[K, V]) Key() K { return n.key }

// Value returns a pointer to the stored value (part of policy.Node interface).
// NOTE: callers must only read/write through this pointer while holding the
// shard lock; otherwise data races may occur.
func (n *node[K, V]) Value() *V { return &n.val }
