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
