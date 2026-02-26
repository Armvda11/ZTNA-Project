package decisioncache

import (
	"testing"
	"time"
)

func TestCachePutGetAndExpire(t *testing.T) {
	c := New(10)
	entry := DecisionEntry{Decision: "allow", Reason: "policy", PolicyVersion: 1}

	c.Put("k1", entry, 50*time.Millisecond)

	got, ok := c.Get("k1", time.Now())
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Decision != "allow" {
		t.Fatalf("unexpected decision: %s", got.Decision)
	}

	time.Sleep(70 * time.Millisecond)
	_, ok = c.Get("k1", time.Now())
	if ok {
		t.Fatal("expected cache miss after expiration")
	}
}

func TestCacheClear(t *testing.T) {
	c := New(10)
	c.Put("k1", DecisionEntry{Decision: "allow"}, time.Minute)
	c.Put("k2", DecisionEntry{Decision: "deny"}, time.Minute)

	c.Clear()

	if _, ok := c.Get("k1", time.Now()); ok {
		t.Fatal("expected k1 to be cleared")
	}
	if _, ok := c.Get("k2", time.Now()); ok {
		t.Fatal("expected k2 to be cleared")
	}
}

func TestCacheMaxEntries(t *testing.T) {
	c := New(1)
	c.Put("k1", DecisionEntry{Decision: "allow"}, time.Minute)
	c.Put("k2", DecisionEntry{Decision: "deny"}, time.Minute)

	_, ok1 := c.Get("k1", time.Now())
	_, ok2 := c.Get("k2", time.Now())
	if !ok1 && !ok2 {
		t.Fatal("expected at least one cache entry to remain")
	}
}
