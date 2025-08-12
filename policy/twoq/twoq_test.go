package twoq

import (
	"testing"

	"github.com/IvanBrykalov/shardcache/policy"
)

// --- test doubles (same shape as in LRU tests) ---

type testNode[K comparable, V any] struct {
	k K
	v V
}

func (n *testNode[K, V]) Key() K    { return n.k }
func (n *testNode[K, V]) Value() *V { return &n.v }

type mockHooks[K comparable, V any] struct {
	pushFrontCnt   int
	moveToFrontCnt int

	lastPush policy.Node[K, V]
	lastMove policy.Node[K, V]
}

func (h *mockHooks[K, V]) MoveToFront(n policy.Node[K, V]) { h.moveToFrontCnt++; h.lastMove = n }
func (h *mockHooks[K, V]) PushFront(n policy.Node[K, V])   { h.pushFrontCnt++; h.lastPush = n }
func (h *mockHooks[K, V]) Remove(policy.Node[K, V])        {}
func (h *mockHooks[K, V]) Back() policy.Node[K, V]         { return nil }
func (h *mockHooks[K, V]) Len() int                        { return 0 }

// --- tests ---

// OnAdd of a first-time key should admit into A1in (no eviction).
func TestTwoQ_AddGoesToA1in(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int](2, 4).New(h).(*twoQ[string, int])

	n1 := &testNode[string, int]{k: "a", v: 1}
	ev := p.OnAdd(n1)

	if ev != nil {
		t.Fatalf("OnAdd should not evict yet")
	}
	if p.inList.Len() != 1 {
		t.Fatalf("A1in must have 1 element, got %d", p.inList.Len())
	}
	if _, ok := p.inIdx[n1]; !ok {
		t.Fatalf("n1 must be present in A1in index")
	}
}

// When A1in overflows, OnAdd should return its LRU candidate.
func TestTwoQ_OverflowReturnsLRUOfA1in(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int](2, 4).New(h).(*twoQ[string, int])

	n1 := &testNode[string, int]{k: "a", v: 1}
	n2 := &testNode[string, int]{k: "b", v: 2}
	n3 := &testNode[string, int]{k: "c", v: 3}

	p.OnAdd(n1)       // A1in: [n1]
	p.OnAdd(n2)       // A1in: [n2, n1] (cap reached)
	ev := p.OnAdd(n3) // A1in: [n3, n2, n1] -> LRU is n1

	if ev == nil || ev != n1 {
		t.Fatalf("expected evict candidate n1 (LRU of A1in), got %v", ev)
	}
}

// Removing a node from A1in should place its key into ghosts (A1out).
func TestTwoQ_OnRemoveFromA1inGoesToGhost(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int](2, 2).New(h).(*twoQ[string, int])

	n1 := &testNode[string, int]{k: "a", v: 1}
	p.OnAdd(n1)
	if _, ok := p.inIdx[n1]; !ok {
		t.Fatal("n1 must be in A1in before removal")
	}
	p.OnRemove(n1)
	if _, ok := p.inIdx[n1]; ok {
		t.Fatal("n1 must be removed from A1in")
	}
	if _, ok := p.ghostIdx["a"]; !ok {
		t.Fatal("key 'a' must be in ghost (A1out)")
	}
}

// Re-admitting a key that is in ghosts should bypass A1in and go to Am.
func TestTwoQ_AddFromGhostGoesToAm(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int](1, 2).New(h).(*twoQ[string, int])

	// 1) Add "a" into A1in and remove -> key goes to A1out.
	n1 := &testNode[string, int]{k: "a", v: 1}
	p.OnAdd(n1)
	p.OnRemove(n1)
	if _, ok := p.ghostIdx["a"]; !ok {
		t.Fatal("key 'a' must be in ghost after removal from A1in")
	}

	// 2) Re-adding "a" should place it directly into Am (not A1in).
	n2 := &testNode[string, int]{k: "a", v: 2}
	ev := p.OnAdd(n2)
	if ev != nil {
		t.Fatalf("OnAdd from ghost must not evict (got %v)", ev)
	}
	if _, ok := p.inIdx[n2]; ok {
		t.Fatalf("n2 must NOT be in A1in (should go to Am)")
	}
}

// A Get on an A1in node should promote it to Am and MoveToFront.
func TestTwoQ_GetPromotesFromA1inToAm(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int](2, 2).New(h).(*twoQ[string, int])

	n1 := &testNode[string, int]{k: "a", v: 1}
	p.OnAdd(n1)
	if _, ok := p.inIdx[n1]; !ok {
		t.Fatal("n1 must be in A1in before Get")
	}
	p.OnGet(n1)
	if _, ok := p.inIdx[n1]; ok {
		t.Fatal("n1 must be promoted out of A1in after Get")
	}
	if h.moveToFrontCnt != 1 {
		t.Fatalf("OnGet must call MoveToFront once")
	}
}
