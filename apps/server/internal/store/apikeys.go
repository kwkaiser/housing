package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrAPIKeyNotFound = errors.New("api key not found")

type APIKey struct {
	ID         int64
	Name       string
	Hint       string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

const apiKeyColumns = `id, name, hint, created_at, coalesce(last_used_at, '')`

func (s *Store) CreateAPIKey(ctx context.Context, name, hint, hash string, at time.Time) (APIKey, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_keys (name, hint, hash, created_at) VALUES (?, ?, ?, ?)`,
		name, hint, hash, timestamp(at))
	if err != nil {
		return APIKey{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return APIKey{}, err
	}
	return s.apiKey(ctx, `WHERE id = ?`, id)
}

func (s *Store) APIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) APIKeyByHash(ctx context.Context, hash string) (APIKey, error) {
	return s.apiKey(ctx, `WHERE hash = ?`, hash)
}

func (s *Store) TouchAPIKey(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, timestamp(at), id)
	return err
}

func (s *Store) DeleteAPIKey(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %d", ErrAPIKeyNotFound, id)
	}
	return nil
}

func (s *Store) apiKey(ctx context.Context, where string, args ...any) (APIKey, error) {
	k, err := scanAPIKey(s.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	return k, err
}

func scanAPIKey(row interface{ Scan(...any) error }) (APIKey, error) {
	var (
		k             APIKey
		created, used string
	)
	if err := row.Scan(&k.ID, &k.Name, &k.Hint, &created, &used); err != nil {
		return APIKey{}, err
	}
	var err error
	if k.CreatedAt, err = parseJobTime(created); err != nil {
		return APIKey{}, err
	}
	if k.LastUsedAt, err = parseJobTime(used); err != nil {
		return APIKey{}, err
	}
	return k, nil
}
