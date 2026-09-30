package config

import (
	"errors"
	"os"
)

const EnvApifyToken = "APIFY_TOKEN"

var ErrMissingApifyToken = errors.New(EnvApifyToken + " is not set")

type Config struct {
	ApifyToken string
}

func Load() (Config, error) {
	token, ok := os.LookupEnv(EnvApifyToken)
	if !ok || token == "" {
		return Config{}, ErrMissingApifyToken
	}
	return Config{ApifyToken: token}, nil
}
