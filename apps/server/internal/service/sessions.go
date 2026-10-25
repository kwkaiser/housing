package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const SessionTTL = 30 * 24 * time.Hour

func (s *Service) CreateSession(ctx context.Context, ss store.Session) (string, error) {
	token := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	now := s.cfg.Now()
	ss.CreatedAt, ss.ExpiresAt = now, now.Add(SessionTTL)
	if err := s.db.CreateSession(ctx, hashToken(token), ss); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Session(ctx context.Context, token string) (store.Session, bool, error) {
	if token == "" {
		return store.Session{}, false, nil
	}
	ss, err := s.db.SessionByHash(ctx, hashToken(token))
	if errors.Is(err, store.ErrSessionNotFound) {
		return store.Session{}, false, nil
	}
	if err != nil {
		return store.Session{}, false, err
	}
	if !s.cfg.Now().Before(ss.ExpiresAt) {
		return store.Session{}, false, nil
	}
	return ss, true, nil
}

func (s *Service) DeleteSession(ctx context.Context, token string) error {
	return s.db.DeleteSession(ctx, hashToken(token))
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}
