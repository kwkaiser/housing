package config

import (
	"errors"
	"os"
)

const (
	EnvApifyToken       = "APIFY_TOKEN"
	EnvOpenRouterAPIKey = "OPENROUTER_API_KEY"
	EnvCartoAPIKey      = "CARTO_API_KEY"
	EnvTransitlandKey   = "TRANSITLAND_API_KEY"

	EnvOIDCIssuer       = "HOUSING_OIDC_ISSUER"
	EnvOIDCClientID     = "HOUSING_OIDC_CLIENT_ID"
	EnvOIDCClientSecret = "HOUSING_OIDC_CLIENT_SECRET"
	EnvPublicURL        = "HOUSING_PUBLIC_URL"
)

var (
	ErrMissingApifyToken       = errors.New(EnvApifyToken + " is not set")
	ErrMissingOpenRouterAPIKey = errors.New(EnvOpenRouterAPIKey + " is not set")
)

type Config struct {
	ApifyToken       string
	OpenRouterAPIKey string
	CartoAPIKey      string
	TransitlandKey   string
}

func Load() Config {
	return Config{
		ApifyToken:       os.Getenv(EnvApifyToken),
		OpenRouterAPIKey: os.Getenv(EnvOpenRouterAPIKey),
		CartoAPIKey:      os.Getenv(EnvCartoAPIKey),
		TransitlandKey:   os.Getenv(EnvTransitlandKey),
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

type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	PublicURL    string
}

func LoadOIDC() OIDC {
	return OIDC{
		Issuer:       os.Getenv(EnvOIDCIssuer),
		ClientID:     os.Getenv(EnvOIDCClientID),
		ClientSecret: os.Getenv(EnvOIDCClientSecret),
		PublicURL:    os.Getenv(EnvPublicURL),
	}
}

func (o OIDC) Missing() []string {
	var out []string
	for _, v := range []struct{ name, value string }{
		{EnvOIDCIssuer, o.Issuer},
		{EnvOIDCClientID, o.ClientID},
		{EnvOIDCClientSecret, o.ClientSecret},
		{EnvPublicURL, o.PublicURL},
	} {
		if v.value == "" {
			out = append(out, v.name)
		}
	}
	return out
}
