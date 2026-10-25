package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	Subject   string
	Name      string
	Email     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s *Store) CreateSession(ctx context.Context, hash string, ss Session) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, timestamp(ss.CreatedAt)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO sessions (hash, subject, name, email, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
			hash, ss.Subject, ss.Name, ss.Email, timestamp(ss.CreatedAt), timestamp(ss.ExpiresAt))
		return err
	})
}

func (s *Store) SessionByHash(ctx context.Context, hash string) (Session, error) {
	var (
		ss               Session
		created, expires string
	)
	err := s.db.QueryRowContext(ctx, `SELECT subject, name, email, created_at, expires_at FROM sessions WHERE hash = ?`, hash).
		Scan(&ss.Subject, &ss.Name, &ss.Email, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if ss.CreatedAt, err = parseJobTime(created); err != nil {
		return Session{}, err
	}
	if ss.ExpiresAt, err = parseJobTime(expires); err != nil {
		return Session{}, err
	}
	return ss, nil
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE hash = ?`, hash)
	return err
}

var ErrHandoffNotFound = errors.New("session handoff not found")

type Handoff struct {
	Challenge string
	Session   Session
	ExpiresAt time.Time
}

func (s *Store) CreateHandoff(ctx context.Context, hash string, h Handoff, now time.Time) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM session_handoffs WHERE expires_at <= ?`, timestamp(now)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO session_handoffs (hash, challenge, subject, name, email, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
			hash, h.Challenge, h.Session.Subject, h.Session.Name, h.Session.Email, timestamp(h.ExpiresAt))
		return err
	})
}

func (s *Store) TakeHandoff(ctx context.Context, hash string) (Handoff, error) {
	var (
		h       Handoff
		expires string
	)
	err := s.tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `DELETE FROM session_handoffs WHERE hash = ? RETURNING challenge, subject, name, email, expires_at`, hash).
			Scan(&h.Challenge, &h.Session.Subject, &h.Session.Name, &h.Session.Email, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHandoffNotFound
		}
		return err
	})
	if err != nil {
		return Handoff{}, err
	}
	if h.ExpiresAt, err = parseJobTime(expires); err != nil {
		return Handoff{}, err
	}
	return h, nil
}
