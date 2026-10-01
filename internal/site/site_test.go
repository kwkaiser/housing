package site

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sample(day string) Site {
	beds := 2
	prev := int64(390000)
	return Site{
		Collections: []Collection{{ID: "somerville", Mode: "rent", Location: "02144", Profiles: []string{"attic"}, Models: []string{"m"}}},
		Profiles:    []Profile{{Collection: "somerville", ID: "attic", Name: "Attic", Wants: []Want{{"Skylights", "essential"}}}},
		Days:        []Day{{Collection: "somerville", Day: day, Listings: 3, Graded: 1}},
		Rows: []Row{{
			Collection: "somerville", Day: day, Source: "zillow", SourceID: "1", URL: "https://example.com/1", Address: "7 Windom St",
			Offer: "rent", PriceCents: 360000, PreviousPriceCents: &prev, FirstSeen: "2026-09-29", Beds: &beds, Profile: "attic",
			Match: 73.4, Score: 73.4, Grades: map[string]Grade{"attic": {Match: 73.4}},
		}},
	}
}

func TestPublish(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	first, err := Publish(ctx, dir, sample("2026-09-30"), now)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "current.json"))
	var cur Current
	if err := json.Unmarshal(b, &cur); err != nil || cur != first || cur.Schema != SchemaVersion {
		t.Fatalf("current.json = %s %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Error("index.html missing")
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, cur.DB)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var version int
	var grades, missing string
	var prev sql.NullInt64
	db.QueryRow("PRAGMA user_version").Scan(&version)
	err = db.QueryRow("SELECT grades, missing, previous_price_cents FROM rows WHERE collection = 'somerville' AND day = '2026-09-30'").Scan(&grades, &missing, &prev)
	var journal string
	db.QueryRow("PRAGMA journal_mode").Scan(&journal)
	db.Close()
	if err != nil || version != SchemaVersion || grades != `{"attic":{"match":73.4,"calibrated":false}}` || missing != "[]" || prev.Int64 != 390000 {
		t.Errorf("row: version=%d grades=%s missing=%s prev=%v err=%v", version, grades, missing, prev, err)
	}
	if journal == "wal" {
		t.Error("the published database must not use WAL; browsers read it as a single file")
	}

	second, err := Publish(ctx, dir, sample("2026-10-01"), now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	third, err := Publish(ctx, dir, sample("2026-10-02"), now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	dbs, _ := filepath.Glob(filepath.Join(dir, "housing-*.sqlite3"))
	if len(dbs) != 2 || second.DB == third.DB {
		t.Errorf("only the current and previous databases should be kept: %v", dbs)
	}
	for _, keep := range []string{second.DB, third.DB} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("%s was pruned", keep)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".publish-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}
