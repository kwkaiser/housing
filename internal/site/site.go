package site

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/renameio/v2"
	_ "modernc.org/sqlite"
)

const SchemaVersion = 1

//go:embed schema.sql
var schema string

//go:embed index.html
var index []byte

type Collection struct {
	ID       string
	Mode     string
	Location string
	Profiles []string
	Models   []string
}

type Want struct {
	Label      string `json:"label"`
	Importance string `json:"importance"`
}

type Avoid struct {
	Label       string `json:"label"`
	Dealbreaker bool   `json:"dealbreaker"`
}

type Reference struct {
	URL     string             `json:"url"`
	Address string             `json:"address"`
	Scores  map[string]float64 `json:"scores"`
}

type Profile struct {
	Collection string
	ID         string
	Name       string
	Summary    string
	Wants      []Want
	Avoids     []Avoid
	References []Reference
}

type Day struct {
	Collection string
	Day        string
	Listings   int
	Graded     int
}

type Grade struct {
	Match      float64 `json:"match"`
	Calibrated bool    `json:"calibrated"`
	Stale      bool    `json:"stale,omitempty"`
}

type Link struct {
	Source     string `json:"source"`
	URL        string `json:"url"`
	PriceCents int64  `json:"price_cents"`
}

type Row struct {
	Collection         string
	Day                string
	Order              int
	Source             string
	SourceID           string
	URL                string
	Address            string
	Offer              string
	PriceCents         int64
	PreviousPriceCents *int64
	FirstSeen          string
	Beds               *int
	Profile            string
	Match              float64
	Calibrated         bool
	Score              float64
	Coverage           float64
	Vibe               float64
	Stale              bool
	Missing            []string
	Dealbreakers       []string
	Avoids             []string
	Summary            string
	Grades             map[string]Grade
	AlsoListed         []Link
}

type Site struct {
	Collections []Collection
	Profiles    []Profile
	Days        []Day
	Rows        []Row
}

type Current struct {
	DB          string    `json:"db"`
	Schema      int       `json:"schema"`
	PublishedAt time.Time `json:"published_at"`
}

const currentFile = "current.json"

func Publish(ctx context.Context, dir string, s Site, now time.Time) (Current, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Current{}, err
	}
	previous, err := readCurrent(dir)
	if err != nil {
		return Current{}, err
	}

	tmp, err := os.CreateTemp(dir, ".publish-*.sqlite3")
	if err != nil {
		return Current{}, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	if err := write(ctx, tmpPath, s, now); err != nil {
		return Current{}, err
	}

	sum, err := hashFile(tmpPath)
	if err != nil {
		return Current{}, err
	}
	cur := Current{DB: "housing-" + sum[:16] + ".sqlite3", Schema: SchemaVersion, PublishedAt: now.UTC()}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return Current{}, err
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, cur.DB)); err != nil {
		return Current{}, err
	}
	if err := renameio.WriteFile(filepath.Join(dir, "index.html"), index, 0o644); err != nil {
		return Current{}, err
	}
	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return Current{}, err
	}
	if err := renameio.WriteFile(filepath.Join(dir, currentFile), append(b, '\n'), 0o644); err != nil {
		return Current{}, err
	}
	return cur, prune(dir, cur.DB, previous.DB)
}

func write(ctx context.Context, path string, s Site, now time.Time) error {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(SchemaVersion)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES ('published_at', ?)`, now.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	for i, c := range s.Collections {
		_, err := tx.ExecContext(ctx, `INSERT INTO collections (id, position, mode, location, profiles, models) VALUES (?, ?, ?, ?, ?, ?)`,
			c.ID, i, c.Mode, c.Location, mustJSON(c.Profiles), mustJSON(c.Models))
		if err != nil {
			return fmt.Errorf("collection %s: %w", c.ID, err)
		}
	}
	for i, p := range s.Profiles {
		_, err := tx.ExecContext(ctx, `INSERT INTO profiles (collection, id, position, name, summary, wants, avoids, refs) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Collection, p.ID, i, p.Name, p.Summary, mustJSON(p.Wants), mustJSON(p.Avoids), mustJSON(p.References))
		if err != nil {
			return fmt.Errorf("profile %s/%s: %w", p.Collection, p.ID, err)
		}
	}
	for _, d := range s.Days {
		if _, err := tx.ExecContext(ctx, `INSERT INTO days (collection, day, listings, graded) VALUES (?, ?, ?, ?)`, d.Collection, d.Day, d.Listings, d.Graded); err != nil {
			return fmt.Errorf("day %s/%s: %w", d.Collection, d.Day, err)
		}
	}
	for _, r := range s.Rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO rows (collection, day, ord, source, source_id, url, address, offer, price_cents, previous_price_cents,
				first_seen, beds, profile, match, calibrated, score, coverage, vibe, stale, missing, dealbreakers, avoids,
				summary, grades, also_listed)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.Collection, r.Day, r.Order, r.Source, r.SourceID, r.URL, r.Address, r.Offer, r.PriceCents, r.PreviousPriceCents,
			r.FirstSeen, r.Beds, r.Profile, r.Match, r.Calibrated, r.Score, r.Coverage, r.Vibe, r.Stale,
			mustJSON(nonNil(r.Missing)), mustJSON(nonNil(r.Dealbreakers)), mustJSON(nonNil(r.Avoids)),
			r.Summary, mustJSON(r.Grades), mustJSON(nonNil(r.AlsoListed)))
		if err != nil {
			return fmt.Errorf("row %s/%s/%s/%s: %w", r.Collection, r.Day, r.Source, r.SourceID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "VACUUM")
	return err
}

func readCurrent(dir string) (Current, error) {
	b, err := os.ReadFile(filepath.Join(dir, currentFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Current{}, nil
	}
	if err != nil {
		return Current{}, err
	}
	var c Current
	if err := json.Unmarshal(b, &c); err != nil {
		return Current{}, fmt.Errorf("decode %s: %w", currentFile, err)
	}
	return c, nil
}

func prune(dir string, keep ...string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "housing-*.sqlite3"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		name := filepath.Base(p)
		if name == keep[0] || name == keep[1] {
			continue
		}
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
