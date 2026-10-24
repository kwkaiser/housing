package store

import (
	"context"
	"errors"
	"fmt"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

var ErrListingNotFound = errors.New("listing not found")

func (s *Store) CurrentListing(ctx context.Context, source listing.Source, sourceID string) (listing.Listing, error) {
	ls, err := s.load(ctx, `
		SELECT l.price_cents, l.currency, l.observed_at, l.url, l.offer, v.data, v.raw, v.collages
		FROM listings l JOIN versions v ON v.id = l.current_version
		WHERE l.source = ? AND l.source_id = ?`, source, sourceID)
	if err != nil {
		return listing.Listing{}, err
	}
	if len(ls) == 0 {
		return listing.Listing{}, fmt.Errorf("%w: %s/%s", ErrListingNotFound, source, sourceID)
	}
	return ls[0], nil
}

func (s *Store) ListingHistory(ctx context.Context, source listing.Source, sourceID string) ([]Sighting, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, price_cents FROM observations WHERE source = ? AND source_id = ? ORDER BY day`, source, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sighting
	for rows.Next() {
		var sg Sighting
		if err := rows.Scan(&sg.Day, &sg.PriceCents); err != nil {
			return nil, err
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}
