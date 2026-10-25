package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	SessionTTL = 30 * 24 * time.Hour
	HandoffTTL = 2 * time.Minute
)

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

func (s *Service) CreateHandoff(ctx context.Context, challenge string, ss store.Session) (string, error) {
	code := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	now := s.cfg.Now()
	if err := s.db.CreateHandoff(ctx, hashToken(code), store.Handoff{Challenge: challenge, Session: ss, ExpiresAt: now.Add(HandoffTTL)}, now); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Service) RedeemHandoff(ctx context.Context, code, verifier string) (string, bool, error) {
	if code == "" || verifier == "" {
		return "", false, nil
	}
	h, err := s.db.TakeHandoff(ctx, hashToken(code))
	if errors.Is(err, store.ErrHandoffNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256([]byte(verifier))
	if !s.cfg.Now().Before(h.ExpiresAt) || subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(h.Challenge)) != 1 {
		return "", false, nil
	}
	token, err := s.CreateSession(ctx, h.Session)
	if err != nil {
		return "", false, err
	}
	return token, true, nil
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
