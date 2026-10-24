package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func (s *Store) SaveProfile(ctx context.Context, p profile.Profile, refs ...listing.Listing) error {
	if err := p.Validate(); err != nil {
		return err
	}
	cols, err := marshalAll(p.Notes, p.Want, p.Avoid, p.Ignore, p.Searches)
	if err != nil {
		return err
	}
	var drafted *string
	if p.Drafted != nil {
		b, err := json.Marshal(p.Drafted)
		if err != nil {
			return err
		}
		d := string(b)
		drafted = &d
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		versions, err := referenceVersions(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		for _, l := range refs {
			v, err := save(ctx, tx, l, false)
			if err != nil {
				return fmt.Errorf("save reference %s/%s: %w", l.Source, l.SourceID, err)
			}
			versions[key(l.Source, l.SourceID)] = v
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO profiles (id, kind, name, summary, notes, want, avoid, ignore, searches, drafted, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				kind = excluded.kind,
				name = excluded.name,
				summary = excluded.summary,
				notes = excluded.notes,
				want = excluded.want,
				avoid = excluded.avoid,
				ignore = excluded.ignore,
				searches = excluded.searches,
				drafted = excluded.drafted,
				updated_at = excluded.updated_at`,
			p.ID, p.Kind, p.Name, p.Summary, cols[0], cols[1], cols[2], cols[3], cols[4], drafted, timestamp(time.Now()))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM profile_references WHERE profile_id = ?`, p.ID); err != nil {
			return err
		}
		for i, r := range p.References {
			if r.Profile != "" || r.Avoid {
				return fmt.Errorf("profile %q: reference %s/%s belongs to profile %q; save the profile, not its effective form", p.ID, r.Source, r.SourceID, r.Profile)
			}
			v, ok := versions[key(r.Source, r.SourceID)]
			if !ok {
				var current sql.NullInt64
				err := tx.QueryRowContext(ctx, `SELECT current_version FROM listings WHERE source = ? AND source_id = ?`, r.Source, r.SourceID).Scan(&current)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if !current.Valid {
					return fmt.Errorf("profile %q: reference listing %s/%s has not been stored", p.ID, r.Source, r.SourceID)
				}
				v = current.Int64
			}
			collages, err := json.Marshal(r.Collages)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO profile_references (profile_id, position, source, source_id, url, collages, version_id)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				p.ID, i, r.Source, r.SourceID, r.URL, string(collages), v)
			if err != nil {
				return fmt.Errorf("profile %q: reference %s/%s: %w", p.ID, r.Source, r.SourceID, err)
			}
		}
		return nil
	})
}

func referenceVersions(ctx context.Context, tx *sql.Tx, profileID string) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT source, source_id, version_id FROM profile_references WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var source, sourceID string
		var v int64
		if err := rows.Scan(&source, &sourceID, &v); err != nil {
			return nil, err
		}
		out[key(listing.Source(source), sourceID)] = v
	}
	return out, rows.Err()
}

func (s *Store) Profile(ctx context.Context, id string) (profile.Profile, error) {
	ps, err := s.profiles(ctx, `WHERE id = ?`, id)
	if err != nil {
		return profile.Profile{}, err
	}
	if len(ps) == 0 {
		return profile.Profile{}, fmt.Errorf("%w: %s", profile.ErrNotFound, id)
	}
	return ps[0], nil
}

func (s *Store) Profiles(ctx context.Context) ([]profile.Profile, error) {
	return s.profiles(ctx, "")
}

func (s *Store) ProfileExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM profiles WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

func (s *Store) ProfileCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM profiles`).Scan(&n)
	return n, err
}

func (s *Store) EffectiveProfile(ctx context.Context, id string) (profile.Profile, error) {
	p, err := s.Profile(ctx, id)
	if err != nil {
		return profile.Profile{}, err
	}
	if p.IsAvoid() {
		return profile.Effective(p, nil)
	}
	all, err := s.Profiles(ctx)
	if err != nil {
		return profile.Profile{}, err
	}
	return profile.Effective(p, all)
}

func (s *Store) profiles(ctx context.Context, where string, args ...any) ([]profile.Profile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, name, summary, notes, want, avoid, ignore, searches, drafted
		FROM profiles `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []profile.Profile
	index := map[string]int{}
	for rows.Next() {
		var (
			p                                    profile.Profile
			notes, want, avoid, ignore, searches string
			drafted                              sql.NullString
		)
		if err := rows.Scan(&p.ID, &p.Kind, &p.Name, &p.Summary, &notes, &want, &avoid, &ignore, &searches, &drafted); err != nil {
			return nil, err
		}
		if err := unmarshalAll([]string{notes, want, avoid, ignore, searches}, &p.Notes, &p.Want, &p.Avoid, &p.Ignore, &p.Searches); err != nil {
			return nil, fmt.Errorf("profile %s: %w", p.ID, err)
		}
		if drafted.Valid {
			if err := json.Unmarshal([]byte(drafted.String), &p.Drafted); err != nil {
				return nil, fmt.Errorf("profile %s: %w", p.ID, err)
			}
		}
		index[p.ID] = len(out)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	refs, err := s.db.QueryContext(ctx, `
		SELECT profile_id, source, source_id, url, collages FROM profile_references
		WHERE profile_id IN (SELECT id FROM profiles `+where+`)
		ORDER BY profile_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer refs.Close()
	for refs.Next() {
		var (
			profileID, collages string
			r                   profile.Reference
		)
		if err := refs.Scan(&profileID, &r.Source, &r.SourceID, &r.URL, &collages); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(collages), &r.Collages); err != nil {
			return nil, err
		}
		if i, ok := index[profileID]; ok {
			out[i].References = append(out[i].References, r)
		}
	}
	if err := refs.Err(); err != nil {
		return nil, err
	}
	for _, p := range out {
		if err := p.Validate(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) ReferenceListings(ctx context.Context, p profile.Profile) ([]listing.Listing, error) {
	out := make([]listing.Listing, 0, len(p.References))
	for _, r := range p.References {
		owner := cmp.Or(r.Profile, p.ID)
		ls, err := s.load(ctx, `
			SELECT l.price_cents, l.currency, l.observed_at, l.url, l.offer, v.data, v.raw, v.collages
			FROM profile_references r
			JOIN versions v ON v.id = r.version_id
			JOIN listings l ON l.source = v.source AND l.source_id = v.source_id
			WHERE r.profile_id = ? AND r.source = ? AND r.source_id = ?`,
			owner, r.Source, r.SourceID)
		if err != nil {
			return nil, err
		}
		if len(ls) == 0 {
			return nil, fmt.Errorf("reference %s/%s of profile %q is not stored", r.Source, r.SourceID, owner)
		}
		out = append(out, ls[0])
	}
	return out, nil
}

func (s *Store) SaveReferenceListings(ctx context.Context, profileID string, ls []listing.Listing) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, l := range ls {
			v, err := save(ctx, tx, l, false)
			if err != nil {
				return fmt.Errorf("save reference %s/%s: %w", l.Source, l.SourceID, err)
			}
			res, err := tx.ExecContext(ctx, `
				UPDATE profile_references SET version_id = ?
				WHERE profile_id = ? AND source = ? AND source_id = ?`,
				v, profileID, l.Source, l.SourceID)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n == 0 {
				return fmt.Errorf("profile %q has no reference %s/%s", profileID, l.Source, l.SourceID)
			}
		}
		return nil
	})
}

func marshalAll(vs ...any) ([]string, error) {
	out := make([]string, len(vs))
	for i, v := range vs {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[i] = string(b)
	}
	return out, nil
}

func unmarshalAll(data []string, vs ...any) error {
	for i, v := range vs {
		if err := json.Unmarshal([]byte(data[i]), v); err != nil {
			return err
		}
	}
	return nil
}
