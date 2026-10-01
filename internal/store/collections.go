package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func (s *Store) SaveCollection(ctx context.Context, c collection.Collection) error {
	if err := c.Validate(); err != nil {
		return err
	}
	cols, err := marshalAll(c.Sources, c.Search)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, id := range c.Profiles {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM profiles WHERE id = ?`, id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("collection %q: %w: %s", c.ID, profile.ErrNotFound, id)
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO collections (id, mode, sources, search, model, max_run_cost_usd, schedule, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				mode = excluded.mode,
				sources = excluded.sources,
				search = excluded.search,
				model = excluded.model,
				max_run_cost_usd = excluded.max_run_cost_usd,
				schedule = excluded.schedule,
				updated_at = excluded.updated_at`,
			c.ID, c.Mode, cols[0], cols[1], c.Model, c.MaxRunCostUSD, nullString(c.Schedule), timestamp(time.Now()))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM collection_profiles WHERE collection_id = ?`, c.ID); err != nil {
			return err
		}
		for i, id := range c.Profiles {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO collection_profiles (collection_id, position, profile_id) VALUES (?, ?, ?)`,
				c.ID, i, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Collection(ctx context.Context, id string) (collection.Collection, error) {
	cs, err := s.collections(ctx, `WHERE id = ?`, id)
	if err != nil {
		return collection.Collection{}, err
	}
	if len(cs) == 0 {
		return collection.Collection{}, fmt.Errorf("%w: %s", collection.ErrNotFound, id)
	}
	return cs[0], nil
}

func (s *Store) Collections(ctx context.Context) ([]collection.Collection, error) {
	return s.collections(ctx, "")
}

func (s *Store) CollectionCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM collections`).Scan(&n)
	return n, err
}

func (s *Store) collections(ctx context.Context, where string, args ...any) ([]collection.Collection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, mode, sources, search, model, max_run_cost_usd, coalesce(schedule, '')
		FROM collections `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []collection.Collection
	index := map[string]int{}
	for rows.Next() {
		var (
			c               collection.Collection
			sources, search string
		)
		if err := rows.Scan(&c.ID, &c.Mode, &sources, &search, &c.Model, &c.MaxRunCostUSD, &c.Schedule); err != nil {
			return nil, err
		}
		if err := unmarshalAll([]string{sources, search}, &c.Sources, &c.Search); err != nil {
			return nil, fmt.Errorf("collection %s: %w", c.ID, err)
		}
		index[c.ID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	members, err := s.db.QueryContext(ctx, `
		SELECT collection_id, profile_id FROM collection_profiles
		WHERE collection_id IN (SELECT id FROM collections `+where+`)
		ORDER BY collection_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer members.Close()
	for members.Next() {
		var collectionID, profileID string
		if err := members.Scan(&collectionID, &profileID); err != nil {
			return nil, err
		}
		if i, ok := index[collectionID]; ok {
			out[i].Profiles = append(out[i].Profiles, profileID)
		}
	}
	if err := members.Err(); err != nil {
		return nil, err
	}
	for _, c := range out {
		if err := c.Validate(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
