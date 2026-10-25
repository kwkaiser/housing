package service

import (
	"crypto/sha256"
	"encoding/base64"
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

func TestHandoffs(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc := openService(t, Config{DataDir: t.TempDir(), Now: func() time.Time { return now }})
	verifier := "verifier-verifier-verifier-verifier-verifie"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	identity := store.Session{Subject: "sub", Name: "Karl", Email: "k@example.com"}

	code, err := svc.CreateHandoff(ctx, challenge, identity)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{"", verifier}, {code, ""}, {"nope", verifier}} {
		if _, ok, err := svc.RedeemHandoff(ctx, bad[0], bad[1]); ok || err != nil {
			t.Errorf("redeem %q: ok=%v err=%v", bad, ok, err)
		}
	}
	token, ok, err := svc.RedeemHandoff(ctx, code, verifier)
	if err != nil || !ok {
		t.Fatalf("redeem = %v %v", ok, err)
	}
	if got, ok, _ := svc.Session(ctx, token); !ok || got.Subject != "sub" || got.Name != "Karl" {
		t.Errorf("handoff session = %+v %v", got, ok)
	}
	if _, ok, _ := svc.RedeemHandoff(ctx, code, verifier); ok {
		t.Error("handoff redeemed twice")
	}

	wrong, _ := svc.CreateHandoff(ctx, challenge, identity)
	if _, ok, _ := svc.RedeemHandoff(ctx, wrong, verifier+"x"); ok {
		t.Error("wrong verifier accepted")
	}
	if _, ok, _ := svc.RedeemHandoff(ctx, wrong, verifier); ok {
		t.Error("handoff usable after a failed redeem")
	}

	late, _ := svc.CreateHandoff(ctx, challenge, identity)
	now = now.Add(HandoffTTL)
	if _, ok, _ := svc.RedeemHandoff(ctx, late, verifier); ok {
		t.Error("expired handoff accepted")
	}
}
