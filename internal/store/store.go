package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"git.kwkaiser.io/kwkaiser/housing/internal/digest"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const (
	FileName  = "housing.sqlite3"
	DayLayout = "2006-01-02"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return userVersion(ctx, s.db)
}

func Day(t time.Time) string {
	return t.Local().Format(DayLayout)
}

func (s *Store) Save(ctx context.Context, ls []listing.Listing) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, l := range ls {
			if _, err := save(ctx, tx, l, false); err != nil {
				return fmt.Errorf("save %s/%s: %w", l.Source, l.SourceID, err)
			}
		}
		return nil
	})
}

func (s *Store) Observe(ctx context.Context, day, collection string, ls []listing.Listing) error {
	if _, err := time.Parse(DayLayout, day); err != nil {
		return fmt.Errorf("invalid day %q: %w", day, err)
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, l := range ls {
			version, err := save(ctx, tx, l, true)
			if err != nil {
				return fmt.Errorf("save %s/%s: %w", l.Source, l.SourceID, err)
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO observations (day, source, source_id, version_id, price_cents, currency, observed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (day, source, source_id) DO UPDATE SET
					version_id = excluded.version_id,
					price_cents = excluded.price_cents,
					currency = excluded.currency,
					observed_at = excluded.observed_at`,
				day, l.Source, l.SourceID, version, l.Price.Cents, l.Price.Currency, timestamp(l.ObservedAt))
			if err != nil {
				return fmt.Errorf("observe %s/%s: %w", l.Source, l.SourceID, err)
			}
			if collection == "" {
				continue
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO collection_observations (collection, day, source, source_id) VALUES (?, ?, ?, ?)
				ON CONFLICT DO NOTHING`,
				collection, day, l.Source, l.SourceID)
			if err != nil {
				return fmt.Errorf("observe %s/%s in %s: %w", l.Source, l.SourceID, collection, err)
			}
		}
		return nil
	})
}

func (s *Store) Load(ctx context.Context) ([]listing.Listing, error) {
	return s.load(ctx, `
		SELECT l.price_cents, l.currency, l.observed_at, l.url, l.offer, v.data, v.raw, v.collages
		FROM listings l JOIN versions v ON v.id = l.current_version
		WHERE EXISTS (SELECT 1 FROM observations o WHERE o.source = l.source AND o.source_id = l.source_id)
		ORDER BY l.source, l.source_id`)
}

func (s *Store) LoadDay(ctx context.Context, day, collection string) ([]listing.Listing, error) {
	return s.load(ctx, `
		SELECT o.price_cents, o.currency, o.observed_at, l.url, l.offer, v.data, v.raw, v.collages
		FROM observations o
		JOIN listings l ON l.source = o.source AND l.source_id = o.source_id
		JOIN versions v ON v.id = o.version_id
		WHERE o.day = ?1 AND (?2 = '' OR EXISTS (
			SELECT 1 FROM collection_observations c
			WHERE c.collection = ?2 AND c.day = o.day AND c.source = o.source AND c.source_id = o.source_id))
		ORDER BY o.source, o.source_id`, day, collection)
}

func (s *Store) LatestDay(ctx context.Context, collection string) (string, error) {
	query := `SELECT max(day) FROM observations`
	args := []any{}
	if collection != "" {
		query = `SELECT max(day) FROM collection_observations WHERE collection = ?`
		args = append(args, collection)
	}
	var day sql.NullString
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&day)
	return day.String, err
}

func (s *Store) load(ctx context.Context, query string, args ...any) ([]listing.Listing, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []listing.Listing
	index := map[string]int{}
	for rows.Next() {
		var (
			l          listing.Listing
			observedAt string
			url, offer string
			data       string
			raw        sql.NullString
			collages   sql.NullString
		)
		if err := rows.Scan(&l.Price.Cents, &l.Price.Currency, &observedAt, &url, &offer, &data, &raw, &collages); err != nil {
			return nil, err
		}
		price := l.Price
		if err := json.Unmarshal([]byte(data), &l); err != nil {
			return nil, err
		}
		l.Price, l.URL, l.Offer = price, url, listing.OfferType(offer)
		if l.ObservedAt, err = time.Parse(time.RFC3339Nano, observedAt); err != nil {
			return nil, err
		}
		if raw.Valid {
			l.Raw = json.RawMessage(raw.String)
		}
		if collages.Valid {
			if err := json.Unmarshal([]byte(collages.String), &l.Collages); err != nil {
				return nil, err
			}
		}
		index[key(l.Source, l.SourceID)] = len(out)
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.loadAssessments(ctx, out, index); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) loadAssessments(ctx context.Context, ls []listing.Listing, index map[string]int) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, source_id, profile_id, data FROM assessments ORDER BY assessed_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, sourceID, profileID, data string
		if err := rows.Scan(&source, &sourceID, &profileID, &data); err != nil {
			return err
		}
		i, ok := index[key(listing.Source(source), sourceID)]
		if !ok {
			continue
		}
		var a listing.Assessment
		if err := json.Unmarshal([]byte(data), &a); err != nil {
			return err
		}
		current := profile.InputHash(ls[i])
		if prev, ok := ls[i].Assessment(profileID, a.Model); ok && prev.InputHash == current && a.InputHash != current {
			continue
		}
		ls[i] = ls[i].WithAssessment(profileID, a)
	}
	return rows.Err()
}

type Run struct {
	Kind       string
	Collection string
	Day        string
	StartedAt  time.Time
	FinishedAt time.Time
	Model      string
	Profiles   []string
	Calls      int
	CostUSD    float64
	Failed     int
	OverBudget int
	Tokens     listing.TokenUsage
	Error      string
}

func (s *Store) RecordRun(ctx context.Context, r Run) error {
	profiles, err := json.Marshal(r.Profiles)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runs (kind, collection, day, started_at, finished_at, model, profiles, calls, cost_usd, failed, over_budget, error,
			prompt_tokens, completion_tokens, reasoning_tokens, cached_tokens)
		VALUES (?, nullif(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Kind, r.Collection, r.Day, timestamp(r.StartedAt), timestamp(r.FinishedAt), r.Model, string(profiles),
		r.Calls, r.CostUSD, r.Failed, r.OverBudget, r.Error,
		r.Tokens.Prompt, r.Tokens.Completion, r.Tokens.Reasoning, r.Tokens.Cached)
	return err
}

func save(ctx context.Context, tx *sql.Tx, l listing.Listing, observed bool) (int64, error) {
	conflict := `DO NOTHING`
	if observed {
		conflict = `DO UPDATE SET
			url = excluded.url,
			offer = excluded.offer,
			price_cents = excluded.price_cents,
			currency = excluded.currency,
			observed_at = excluded.observed_at`
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO listings (source, source_id, url, offer, price_cents, currency, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source, source_id) `+conflict,
		l.Source, l.SourceID, l.URL, l.Offer, l.Price.Cents, l.Price.Currency, timestamp(l.ObservedAt))
	if err != nil {
		return 0, err
	}

	data, err := json.Marshal(content(l))
	if err != nil {
		return 0, err
	}
	var collages, inputHash, raw *string
	if len(l.Collages) > 0 {
		b, err := json.Marshal(l.Collages)
		if err != nil {
			return 0, err
		}
		c, h := string(b), profile.InputHash(l)
		collages, inputHash = &c, &h
	}
	if len(l.Raw) > 0 {
		r := string(l.Raw)
		raw = &r
	}
	var lat, lng *float64
	if l.Coordinates != nil {
		lat, lng = &l.Coordinates.Lat, &l.Coordinates.Lng
	}

	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO versions (source, source_id, content_hash, input_hash, address, city, state, postal_code,
			lat, lng, beds, baths, sqft, collages, data, raw, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source, source_id, content_hash) DO UPDATE SET
			data = excluded.data,
			raw = coalesce(excluded.raw, versions.raw),
			collages = coalesce(excluded.collages, versions.collages),
			input_hash = coalesce(excluded.input_hash, versions.input_hash)
		RETURNING id`,
		l.Source, l.SourceID, ContentHash(l), inputHash, l.Address.Formatted, l.Address.City, l.Address.State, l.Address.PostalCode,
		lat, lng, l.Beds, l.Baths, l.SqFt, collages, string(data), raw, timestamp(time.Now()),
	).Scan(&version)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE listings SET current_version = ?
		WHERE source = ? AND source_id = ? AND (? OR current_version IS NULL)`,
		version, l.Source, l.SourceID, observed); err != nil {
		return 0, err
	}

	for profileID, byModel := range l.Assessments {
		for model, a := range byModel {
			b, err := json.Marshal(a)
			if err != nil {
				return 0, err
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO assessments (source, source_id, profile_id, model, profile_hash, input_hash,
					score, coverage, assessed_at, cost_usd, data)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (source, source_id, profile_id, model, profile_hash, input_hash) DO UPDATE SET
					score = excluded.score,
					coverage = excluded.coverage,
					assessed_at = excluded.assessed_at,
					cost_usd = excluded.cost_usd,
					data = excluded.data`,
				l.Source, l.SourceID, profileID, model, a.ProfileHash, a.InputHash,
				a.Score, a.Coverage, timestamp(a.AssessedAt), a.CostUSD, string(b))
			if err != nil {
				return 0, err
			}
		}
	}
	return version, nil
}

func content(l listing.Listing) listing.Listing {
	l.URL = ""
	l.Price = listing.Money{}
	l.ObservedAt = time.Time{}
	l.Collages = nil
	l.Assessments = nil
	l.Raw = nil
	return l
}

func ContentHash(l listing.Listing) string {
	c := content(l)
	c.Photos = make([]string, len(l.Photos))
	for i, u := range l.Photos {
		c.Photos[i] = media.PhotoID(u)
	}
	b, _ := json.Marshal(c)
	return digest.Hex(b)
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func key(source listing.Source, sourceID string) string {
	return string(source) + "/" + sourceID
}

func timestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

type DayCount struct {
	Day      string
	Listings int
}

func (s *Store) CollectionDays(ctx context.Context, collection string) ([]DayCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, count(*) FROM collection_observations WHERE collection = ? GROUP BY day ORDER BY day`, collection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayCount
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Listings); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type Sighting struct {
	Day        string
	PriceCents int64
}

func (s *Store) History(ctx context.Context, collection string) (map[string][]Sighting, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.source, o.source_id, o.day, o.price_cents FROM observations o
		WHERE EXISTS (SELECT 1 FROM collection_observations c
			WHERE c.collection = ? AND c.source = o.source AND c.source_id = o.source_id)
		ORDER BY o.day`, collection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Sighting{}
	for rows.Next() {
		var source, sourceID string
		var sg Sighting
		if err := rows.Scan(&source, &sourceID, &sg.Day, &sg.PriceCents); err != nil {
			return nil, err
		}
		k := key(listing.Source(source), sourceID)
		out[k] = append(out[k], sg)
	}
	return out, rows.Err()
}

func Key(l listing.Listing) string {
	return key(l.Source, l.SourceID)
}
