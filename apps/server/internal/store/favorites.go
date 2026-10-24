package store

import (
	"context"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type Favorite struct {
	Listing   listing.Listing
	CreatedAt time.Time
	LastSeen  string
}

func (s *Store) AddFavorite(ctx context.Context, source listing.Source, sourceID string, at time.Time) error {
	if _, err := s.CurrentListing(ctx, source, sourceID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO favorites (source, source_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		source, sourceID, timestamp(at))
	return err
}

func (s *Store) RemoveFavorite(ctx context.Context, source listing.Source, sourceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE source = ? AND source_id = ?`, source, sourceID)
	return err
}

func (s *Store) IsFavorite(ctx context.Context, source listing.Source, sourceID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM favorites WHERE source = ? AND source_id = ?`, source, sourceID).Scan(&n)
	return n > 0, err
}

func (s *Store) FavoriteKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, source_id FROM favorites`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var source, sourceID string
		if err := rows.Scan(&source, &sourceID); err != nil {
			return nil, err
		}
		out[key(listing.Source(source), sourceID)] = true
	}
	return out, rows.Err()
}

func (s *Store) Favorites(ctx context.Context) ([]Favorite, error) {
	ls, err := s.load(ctx, `
		SELECT l.price_cents, l.currency, l.observed_at, l.url, l.offer, v.data, v.raw, v.collages
		FROM favorites f
		JOIN listings l ON l.source = f.source AND l.source_id = f.source_id
		JOIN versions v ON v.id = l.current_version
		ORDER BY f.created_at DESC, f.source, f.source_id`)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.source, f.source_id, f.created_at, coalesce(max(o.day), '')
		FROM favorites f LEFT JOIN observations o ON o.source = f.source AND o.source_id = f.source_id
		GROUP BY f.source, f.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type meta struct {
		created  time.Time
		lastSeen string
	}
	metas := map[string]meta{}
	for rows.Next() {
		var source, sourceID, created, lastSeen string
		if err := rows.Scan(&source, &sourceID, &created, &lastSeen); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		metas[key(listing.Source(source), sourceID)] = meta{t, lastSeen}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Favorite, len(ls))
	for i, l := range ls {
		m := metas[Key(l)]
		out[i] = Favorite{Listing: l, CreatedAt: m.created, LastSeen: m.lastSeen}
	}
	return out, nil
}
