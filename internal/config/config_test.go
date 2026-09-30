package config

import (
	"errors"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv(EnvApifyToken, "token")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ApifyToken != "token" {
		t.Fatalf("got %q", cfg.ApifyToken)
	}
}

func TestLoadMissing(t *testing.T) {
	t.Setenv(EnvApifyToken, "")
	if _, err := Load(); !errors.Is(err, ErrMissingApifyToken) {
		t.Fatalf("got %v", err)
	}
}
