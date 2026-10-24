package config

import (
	"errors"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv(EnvApifyToken, "apify")
	t.Setenv(EnvOpenRouterAPIKey, "router")
	cfg := Load()
	if got, err := cfg.Apify(); err != nil || got != "apify" {
		t.Fatalf("Apify() = %q, %v", got, err)
	}
	if got, err := cfg.OpenRouter(); err != nil || got != "router" {
		t.Fatalf("OpenRouter() = %q, %v", got, err)
	}
}

func TestLoadMissing(t *testing.T) {
	t.Setenv(EnvApifyToken, "")
	t.Setenv(EnvOpenRouterAPIKey, "")
	cfg := Load()
	if _, err := cfg.Apify(); !errors.Is(err, ErrMissingApifyToken) {
		t.Fatalf("got %v", err)
	}
	if _, err := cfg.OpenRouter(); !errors.Is(err, ErrMissingOpenRouterAPIKey) {
		t.Fatalf("got %v", err)
	}
}
