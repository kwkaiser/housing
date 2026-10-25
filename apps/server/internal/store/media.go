package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type MediaRef struct {
	Photos   []string
	Collages []string
}

func (s *Store) MediaRefs(ctx context.Context, since time.Time) ([]MediaRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT json_extract(v.data, '$.photos'), v.collages FROM versions v
		WHERE v.id IN (
			SELECT current_version FROM listings WHERE observed_at >= ?1
			UNION SELECT version_id FROM observations WHERE day >= ?2
			UNION SELECT v.id FROM versions v JOIN favorites f ON f.source = v.source AND f.source_id = v.source_id
			UNION SELECT version_id FROM profile_references
		)
		UNION ALL
		SELECT NULL, collages FROM profile_references`,
		timestamp(since), Day(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaRef
	for rows.Next() {
		var (
			r                MediaRef
			photos, collages sql.NullString
		)
		if err := rows.Scan(&photos, &collages); err != nil {
			return nil, err
		}
		if photos.Valid {
			if err := json.Unmarshal([]byte(photos.String), &r.Photos); err != nil {
				return nil, err
			}
		}
		if collages.Valid {
			if err := json.Unmarshal([]byte(collages.String), &r.Collages); err != nil {
				return nil, err
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
