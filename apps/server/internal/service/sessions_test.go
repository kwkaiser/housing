package service

import (
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func TestSessions(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc := openService(t, Config{DataDir: t.TempDir(), Now: func() time.Time { return now }})

	token, err := svc.CreateSession(ctx, store.Session{Subject: "sub", Name: "Karl", Email: "k@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "nope", token + "x"} {
		if _, ok, err := svc.Session(ctx, bad); ok || err != nil {
			t.Errorf("token %q: ok=%v err=%v", bad, ok, err)
		}
	}
	got, ok, err := svc.Session(ctx, token)
	if err != nil || !ok || got.Subject != "sub" || got.Name != "Karl" || !got.ExpiresAt.Equal(now.Add(SessionTTL)) {
		t.Fatalf("session = %+v %v %v", got, ok, err)
	}

	now = now.Add(SessionTTL)
	if _, ok, _ := svc.Session(ctx, token); ok {
		t.Error("expired session still valid")
	}
	if _, err := svc.CreateSession(ctx, store.Session{Subject: "other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.SessionByHash(ctx, hashToken(token)); err != store.ErrSessionNotFound {
		t.Errorf("expired session not pruned: %v", err)
	}

	fresh, err := svc.CreateSession(ctx, store.Session{Subject: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSession(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := svc.Session(ctx, fresh); ok {
		t.Error("deleted session still valid")
	}
}
