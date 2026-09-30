package config

import (
	"errors"
	"os"
)

const (
	EnvApifyToken       = "APIFY_TOKEN"
	EnvOpenRouterAPIKey = "OPENROUTER_API_KEY"
)

var (
	ErrMissingApifyToken       = errors.New(EnvApifyToken + " is not set")
	ErrMissingOpenRouterAPIKey = errors.New(EnvOpenRouterAPIKey + " is not set")
)

type Config struct {
	ApifyToken       string
	OpenRouterAPIKey string
}

func Load() Config {
	return Config{
		ApifyToken:       os.Getenv(EnvApifyToken),
		OpenRouterAPIKey: os.Getenv(EnvOpenRouterAPIKey),
	}
}

func (c Config) Apify() (string, error) {
	if c.ApifyToken == "" {
		return "", ErrMissingApifyToken
	}
	return c.ApifyToken, nil
}

func (c Config) OpenRouter() (string, error) {
	if c.OpenRouterAPIKey == "" {
		return "", ErrMissingOpenRouterAPIKey
	}
	return c.OpenRouterAPIKey, nil
}
