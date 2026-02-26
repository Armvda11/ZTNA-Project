package crl

import (
	"context"
	"testing"
)

func TestStoreReplaceAndIsRevoked(t *testing.T) {
	s := NewStore()
	s.Replace([]string{"aa", "bb"})

	if !s.IsRevoked("aa") {
		t.Fatal("expected aa to be revoked")
	}
	if s.IsRevoked("cc") {
		t.Fatal("did not expect cc to be revoked")
	}
}

func TestStoreSnapshotIsCopy(t *testing.T) {
	s := NewStore()
	s.Replace([]string{"aa"})

	snap := s.Snapshot()
	delete(snap, "aa")

	if !s.IsRevoked("aa") {
		t.Fatal("store should be unaffected by snapshot mutations")
	}
}

func TestStartAutoRefresh_NoOp(t *testing.T) {
	s := NewStore()
	if err := s.StartAutoRefresh(context.Background()); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}
