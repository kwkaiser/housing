package store

import (
	"errors"
	"testing"
	"time"
)

func TestAPIKeys(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	at := time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)

	a, err := s.CreateAPIKey(ctx, "agent", "hk_abcdef", "hash-a", at)
	if err != nil || a.ID == 0 || a.Name != "agent" || a.Hint != "hk_abcdef" || !a.CreatedAt.Equal(at) || !a.LastUsedAt.IsZero() {
		t.Fatalf("create = %+v %v", a, err)
	}
	if _, err := s.CreateAPIKey(ctx, "dup", "hk_abcdef", "hash-a", at); err == nil {
		t.Error("duplicate hash should be rejected")
	}
	b, err := s.CreateAPIKey(ctx, "script", "hk_123456", "hash-b", at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if got, err := s.APIKeyByHash(ctx, "hash-b"); err != nil || got.ID != b.ID {
		t.Errorf("by hash = %+v %v", got, err)
	}
	if _, err := s.APIKeyByHash(ctx, "nope"); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Errorf("missing hash err = %v", err)
	}
	if err := s.TouchAPIKey(ctx, a.ID, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ks, err := s.APIKeys(ctx)
	if err != nil || len(ks) != 2 || ks[0].ID != a.ID || !ks[0].LastUsedAt.Equal(at.Add(time.Minute)) || ks[1].ID != b.ID {
		t.Errorf("list = %+v %v", ks, err)
	}

	if err := s.DeleteAPIKey(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAPIKey(ctx, a.ID); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Errorf("delete again err = %v", err)
	}
	if _, err := s.APIKeyByHash(ctx, "hash-a"); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Errorf("deleted key still found: %v", err)
	}
}
