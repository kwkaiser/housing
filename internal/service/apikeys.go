package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	APIKeyPrefix     = "hk_"
	apiKeyHintLength = len(APIKeyPrefix) + 6
	apiKeyNameMax    = 100
	apiKeyTouchEvery = time.Minute
)

func (s *Service) CreateAPIKey(ctx context.Context, name string) (store.APIKey, string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return store.APIKey{}, "", profile.FieldError{Field: "name", Err: errors.New("a name is required")}
	case utf8.RuneCountInString(name) > apiKeyNameMax:
		return store.APIKey{}, "", profile.FieldError{Field: "name", Err: errors.New("the name must be at most 100 characters")}
	}
	token := APIKeyPrefix + base64.RawURLEncoding.EncodeToString(randomBytes(32))
	k, err := s.db.CreateAPIKey(ctx, name, token[:apiKeyHintLength], hashAPIKey(token), s.cfg.Now())
	if err != nil {
		return store.APIKey{}, "", err
	}
	return k, token, nil
}

func (s *Service) APIKeys(ctx context.Context) ([]store.APIKey, error) {
	return s.db.APIKeys(ctx)
}

func (s *Service) DeleteAPIKey(ctx context.Context, id int64) error {
	return s.db.DeleteAPIKey(ctx, id)
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, token string) (store.APIKey, bool, error) {
	if !strings.HasPrefix(token, APIKeyPrefix) {
		return store.APIKey{}, false, nil
	}
	k, err := s.db.APIKeyByHash(ctx, hashAPIKey(token))
	if errors.Is(err, store.ErrAPIKeyNotFound) {
		return store.APIKey{}, false, nil
	}
	if err != nil {
		return store.APIKey{}, false, err
	}
	if now := s.cfg.Now(); now.Sub(k.LastUsedAt) >= apiKeyTouchEvery {
		if err := s.db.TouchAPIKey(ctx, k.ID, now); err != nil {
			return store.APIKey{}, false, err
		}
		k.LastUsedAt = now
	}
	return k, true, nil
}

func hashAPIKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}
