package lru

import (
	"testing"

	"github.com/IvanBrykalov/shardcache/policy"
)

// --- test doubles ---

type testNode[K comparable, V any] struct {
	k K
	v V
}

func (n *testNode[K, V]) Key() K    { return n.k }
func (n *testNode[K, V]) Value() *V { return &n.v }

type mockHooks[K comparable, V any] struct {
	pushFrontCnt   int
	moveToFrontCnt int
	removeCnt      int

	lastPush policy.Node[K, V]
	lastMove policy.Node[K, V]
	lastRem  policy.Node[K, V]

	lenVal  int
	backVal policy.Node[K, V]
}

func (h *mockHooks[K, V]) MoveToFront(n policy.Node[K, V]) { h.moveToFrontCnt++; h.lastMove = n }
func (h *mockHooks[K, V]) PushFront(n policy.Node[K, V])   { h.pushFrontCnt++; h.lastPush = n }
func (h *mockHooks[K, V]) Remove(n policy.Node[K, V])      { h.removeCnt++; h.lastRem = n }
func (h *mockHooks[K, V]) Back() policy.Node[K, V]         { return h.backVal }
func (h *mockHooks[K, V]) Len() int                        { return h.lenVal }

// --- tests ---

// OnAdd should push the node to MRU and never propose an eviction.
func TestLRU_OnAdd_PushFrontAndNoEvict(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int]().New(h) // shard-local policy

	n := &testNode[string, int]{k: "k1", v: 1}
	ev := p.OnAdd(n)

	if ev != nil {
		t.Fatalf("OnAdd must not return evict candidate for LRU, got %v", ev)
	}
	if h.pushFrontCnt != 1 || h.lastPush != n {
		t.Fatalf("OnAdd must call PushFront exactly once with the node")
	}
	if h.moveToFrontCnt != 0 || h.removeCnt != 0 {
		t.Fatalf("OnAdd must not call MoveToFront/Remove")
	}
}

// OnGet should promote the node to MRU.
func TestLRU_OnGet_MoveToFront(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int]().New(h)

	n := &testNode[string, int]{k: "k2", v: 2}
	p.OnGet(n)

	if h.moveToFrontCnt != 1 || h.lastMove != n {
		t.Fatalf("OnGet must call MoveToFront exactly once with the node")
	}
	if h.pushFrontCnt != 0 || h.removeCnt != 0 {
		t.Fatalf("OnGet must not call PushFront/Remove")
	}
}

// OnUpdate should promote the node to MRU (updates count as recent use).
func TestLRU_OnUpdate_MoveToFront(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int]().New(h)

	n := &testNode[string, int]{k: "k3", v: 3}
	p.OnUpdate(n)

	if h.moveToFrontCnt != 1 || h.lastMove != n {
		t.Fatalf("OnUpdate must call MoveToFront exactly once with the node")
	}
	if h.pushFrontCnt != 0 || h.removeCnt != 0 {
		t.Fatalf("OnUpdate must not call PushFront/Remove")
	}
}

// OnRemove is a no-op for pure LRU.
func TestLRU_OnRemove_NoOp(t *testing.T) {
	t.Parallel()

	h := &mockHooks[string, int]{}
	p := New[string, int]().New(h)

	n := &testNode[string, int]{k: "k4", v: 4}
	p.OnRemove(n)

	if h.pushFrontCnt != 0 || h.moveToFrontCnt != 0 || h.removeCnt != 0 {
		t.Fatalf("OnRemove for LRU must be no-op (no hooks should be called)")
	}
}
