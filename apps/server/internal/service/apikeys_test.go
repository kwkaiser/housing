package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func TestAPIKeys(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc := openService(t, Config{DataDir: t.TempDir(), Now: func() time.Time { return now }})

	for _, name := range []string{"", "   ", strings.Repeat("x", 101)} {
		var fe profile.FieldError
		if _, _, err := svc.CreateAPIKey(ctx, name); !errors.As(err, &fe) || fe.Field != "name" {
			t.Errorf("name %q: err = %v", name, err)
		}
	}

	k, token, err := svc.CreateAPIKey(ctx, "  agent  ")
	if err != nil {
		t.Fatal(err)
	}
	if k.Name != "agent" || !strings.HasPrefix(token, APIKeyPrefix) || len(token) != len(APIKeyPrefix)+43 || k.Hint != token[:9] {
		t.Errorf("created %+v %q", k, token)
	}
	_, other, err := svc.CreateAPIKey(ctx, "agent")
	if err != nil || other == token {
		t.Fatalf("second key %q %v", other, err)
	}

	for _, bad := range []string{"", token[:20], token + "x", strings.TrimPrefix(token, APIKeyPrefix)} {
		if _, ok, err := svc.AuthenticateAPIKey(ctx, bad); ok || err != nil {
			t.Errorf("token %q: ok=%v err=%v", bad, ok, err)
		}
	}

	got, ok, err := svc.AuthenticateAPIKey(ctx, token)
	if err != nil || !ok || got.ID != k.ID || !got.LastUsedAt.Equal(now) {
		t.Fatalf("auth = %+v %v %v", got, ok, err)
	}
	first := now
	now = now.Add(30 * time.Second)
	if got, _, _ := svc.AuthenticateAPIKey(ctx, token); !got.LastUsedAt.Equal(first) {
		t.Errorf("last used within a minute should not move: %v", got.LastUsedAt)
	}
	now = first.Add(time.Minute)
	if got, _, _ := svc.AuthenticateAPIKey(ctx, token); !got.LastUsedAt.Equal(now) {
		t.Errorf("last used after a minute = %v, want %v", got.LastUsedAt, now)
	}

	if err := svc.DeleteAPIKey(ctx, k.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := svc.AuthenticateAPIKey(ctx, token); ok {
		t.Error("deleted key still authenticates")
	}
	if _, ok, _ := svc.AuthenticateAPIKey(ctx, other); !ok {
		t.Error("deleting one key should leave the other")
	}
}
