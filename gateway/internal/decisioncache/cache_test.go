package decisioncache

import (
	"testing"
	"time"

	"ztna-gateway/internal/pep"
)

func TestCachePutGet(t *testing.T) {
	c := New(10)
	key := "k"
	want := pep.AuthorizeResponse{DecisionID: "dec-1", Effect: "allow", PolicyVersion: 1}
	c.Put(key, want, 2*time.Second)

	got, ok := c.Get(key, time.Now())
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.DecisionID != want.DecisionID {
		t.Fatalf("got decision_id=%q want=%q", got.DecisionID, want.DecisionID)
	}
}

func TestInvalidateOnPolicyChange(t *testing.T) {
	c := New(10)
	key := "k"
	c.Put(key, pep.AuthorizeResponse{DecisionID: "dec-1", PolicyVersion: 1}, time.Minute)
	c.InvalidateOnPolicyChange(1)
	if _, ok := c.Get(key, time.Now()); !ok {
		t.Fatal("expected entry to remain when policy version unchanged")
	}

	c.InvalidateOnPolicyChange(2)
	if _, ok := c.Get(key, time.Now()); ok {
		t.Fatal("expected cache miss after policy version change")
	}
}
