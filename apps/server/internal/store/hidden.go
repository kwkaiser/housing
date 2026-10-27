package store

import (
	"context"
	"fmt"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func (s *Store) SetHidden(ctx context.Context, source listing.Source, sourceID string, on bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE listings SET hidden = ? WHERE source = ? AND source_id = ?`, on, source, sourceID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: %s/%s", ErrListingNotFound, source, sourceID)
	}
	return nil
}

func (s *Store) IsHidden(ctx context.Context, source listing.Source, sourceID string) (bool, error) {
	var on bool
	err := s.db.QueryRowContext(ctx, `SELECT coalesce(max(hidden), 0) FROM listings WHERE source = ? AND source_id = ?`, source, sourceID).Scan(&on)
	return on, err
}

func (s *Store) HiddenKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source, source_id FROM listings WHERE hidden`)
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
