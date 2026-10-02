package store

import (
	"path/filepath"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample() listing.Listing {
	beds := 2
	return listing.Listing{
		Source:      listing.SourceFacebook,
		SourceID:    "1",
		URL:         "https://example.com/1",
		Offer:       listing.OfferRent,
		Price:       listing.Money{Cents: 280000, Currency: "USD"},
		Address:     listing.Address{Formatted: "23 Everett St, Cambridge, MA", Street: "23 Everett St", City: "Cambridge", PostalCode: "02138"},
		Coordinates: &listing.Coordinates{Lat: 42.38, Lng: -71.11},
		Beds:        &beds,
		Description: "sunny",
		Photos:      []string{"https://scontent-a.xx.fbcdn.net/v/1_n.jpg?oe=AAA"},
		ObservedAt:  time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
		Raw:         []byte(`{"id":"1"}`),
	}
}

func count(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	ctx := t.Context()
	for range 2 {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		ms, _ := migrations()
		if v, err := s.SchemaVersion(ctx); err != nil || v != len(ms) {
			t.Errorf("version = %d %v, want %d", v, err, len(ms))
		}
		s.Close()
	}

	s, _ := Open(ctx, path)
	s.db.Exec("PRAGMA user_version = 999")
	s.Close()
	if _, err := Open(ctx, path); err == nil {
		t.Error("a database newer than the binary should be refused")
	}
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	l := sample()
	l.Collages = []string{"collages/ab/c/0.jpg"}
	l = l.WithAssessment("attic", listing.Assessment{Model: "m", Score: 40, ProfileHash: "p1", InputHash: "i1", AssessedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})
	if err := s.Observe(ctx, "2026-09-30", "", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}
	newer := l.WithAssessment("attic", listing.Assessment{Model: "m", Score: 70, ProfileHash: "p2", InputHash: "i1", AssessedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)})
	if err := s.Save(ctx, []listing.Listing{newer}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("load: %v %+v", err, got)
	}
	g := got[0]
	if g.URL != l.URL || g.Price != l.Price || g.Description != "sunny" || *g.Beds != 2 || g.Coordinates.Lng != -71.11 ||
		!g.ObservedAt.Equal(l.ObservedAt) || string(g.Raw) != `{"id":"1"}` || len(g.Collages) != 1 || g.Photos[0] != l.Photos[0] {
		t.Errorf("round trip lost data: %+v", g)
	}
	if a, ok := g.Assessment("attic", "m"); !ok || a.Score != 70 {
		t.Errorf("load should return the latest assessment: %+v", a)
	}
	if n := count(t, s, "SELECT count(*) FROM assessments"); n != 2 {
		t.Errorf("assessment history should be kept: %d rows", n)
	}
}

func TestVersions(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	l := sample()
	l.Collages = []string{"collages/ab/c/0.jpg"}
	if err := s.Observe(ctx, "2026-09-29", "", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}

	next := sample()
	next.Price.Cents = 270000
	next.Photos = []string{"https://scontent-b.xx.fbcdn.net/v/1_n.jpg?oe=BBB"}
	next.ObservedAt = next.ObservedAt.AddDate(0, 0, 1)
	if err := s.Observe(ctx, "2026-09-30", "", []listing.Listing{next}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM versions"); n != 1 {
		t.Errorf("a price change or refreshed photo url should not create a version: %d", n)
	}
	if n := count(t, s, "SELECT count(*) FROM observations WHERE price_cents = 270000 AND day = '2026-09-30'"); n != 1 {
		t.Error("each day's price should be observed")
	}
	got, _ := s.Load(ctx)
	if len(got[0].Collages) != 1 || got[0].Price.Cents != 270000 || got[0].Photos[0] != next.Photos[0] {
		t.Errorf("saving without collages should keep them, and take the latest price and urls: %+v", got[0])
	}

	changed := next
	changed.Description = "sunny, renovated"
	if err := s.Observe(ctx, "2026-10-01", "", []listing.Listing{changed}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM versions"); n != 2 {
		t.Errorf("a description change should create a version: %d", n)
	}
	if n := count(t, s, "SELECT count(DISTINCT version_id) FROM observations"); n != 2 {
		t.Errorf("observations should point at the version seen that day: %d", n)
	}
	got, _ = s.Load(ctx)
	if got[0].Description != "sunny, renovated" || got[0].Collages != nil {
		t.Errorf("load should return the current version: %+v", got[0])
	}
	if err := s.Observe(ctx, "yesterday", "", nil); err == nil {
		t.Error("invalid day should be rejected")
	}
}

func TestDays(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	if day, err := s.LatestDay(ctx, ""); err != nil || day != "" {
		t.Errorf("empty store latest day = %q %v", day, err)
	}
	old := sample()
	old.Collages = []string{"collages/old/0.jpg"}
	if err := s.Observe(ctx, "2026-09-29", "", []listing.Listing{old}); err != nil {
		t.Fatal(err)
	}
	newer := sample()
	newer.Description = "renovated"
	newer.Price.Cents = 300000
	newer.Collages = []string{"collages/new/0.jpg"}
	if err := s.Observe(ctx, "2026-09-30", "", []listing.Listing{newer}); err != nil {
		t.Fatal(err)
	}

	gradeOld := old.WithAssessment("attic", listing.Assessment{Model: "m", Score: 10, InputHash: profile.InputHash(old), AssessedAt: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)})
	if err := s.Save(ctx, []listing.Listing{gradeOld}); err != nil {
		t.Fatal(err)
	}
	gradeNew := newer.WithAssessment("attic", listing.Assessment{Model: "m", Score: 90, InputHash: profile.InputHash(newer), AssessedAt: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)})
	if err := s.Save(ctx, []listing.Listing{gradeNew}); err != nil {
		t.Fatal(err)
	}

	if day, _ := s.LatestDay(ctx, ""); day != "2026-09-30" {
		t.Errorf("latest day = %s", day)
	}
	past, err := s.LoadDay(ctx, "2026-09-29", "")
	if err != nil || len(past) != 1 {
		t.Fatalf("load day: %v %+v", err, past)
	}
	if past[0].Description != "sunny" || past[0].Price.Cents != 280000 {
		t.Errorf("a past day should show that day's version and price: %+v", past[0])
	}
	if a, _ := past[0].Assessment("attic", "m"); a.Score != 10 {
		t.Errorf("a past day should keep the grade made for its version: %+v", a)
	}
	current, _ := s.Load(ctx)
	if current[0].Description != "renovated" || current[0].Price.Cents != 300000 {
		t.Errorf("saving an older version must not roll back the listing: %+v", current[0])
	}
	if a, _ := current[0].Assessment("attic", "m"); a.Score != 90 {
		t.Errorf("current listing should use its own grade: %+v", a)
	}
	if none, _ := s.LoadDay(ctx, "2026-01-01", ""); len(none) != 0 {
		t.Error("a day without observations should be empty")
	}
}

func TestRecordRun(t *testing.T) {
	s := open(t)
	now := time.Now()
	if err := s.RecordRun(t.Context(), Run{Kind: "assess", Day: "2026-09-30", StartedAt: now, FinishedAt: now, Profiles: []string{"attic"}, Calls: 3, CostUSD: 0.06}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM runs WHERE cost_usd = 0.06 AND profiles = '[\"attic\"]'"); n != 1 {
		t.Error("run not recorded")
	}
	tokens := listing.TokenUsage{Prompt: 9000, Completion: 1200, Reasoning: 700, Cached: 3000}
	if err := s.RecordRun(t.Context(), Run{Kind: "assess", Day: "2026-10-01", StartedAt: now, FinishedAt: now, Tokens: tokens}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM runs WHERE prompt_tokens = 9000 AND completion_tokens = 1200 AND reasoning_tokens = 700 AND cached_tokens = 3000"); n != 1 {
		t.Error("run tokens not recorded")
	}
}

func TestCollections(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	a, b := sample(), sample()
	b.SourceID = "2"
	if err := s.Observe(ctx, "2026-09-30", "somerville", []listing.Listing{a}); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe(ctx, "2026-09-30", "austin", []listing.Listing{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe(ctx, "2026-10-01", "austin", []listing.Listing{b}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadDay(ctx, "2026-09-30", "somerville"); len(got) != 1 || got[0].SourceID != "1" {
		t.Errorf("collection day should only hold its own listings: %+v", got)
	}
	if got, _ := s.LoadDay(ctx, "2026-09-30", ""); len(got) != 2 {
		t.Errorf("no collection should load every listing: %d", len(got))
	}
	if day, _ := s.LatestDay(ctx, "somerville"); day != "2026-09-30" {
		t.Errorf("latest somerville day = %s", day)
	}
	if day, _ := s.LatestDay(ctx, "austin"); day != "2026-10-01" {
		t.Errorf("latest austin day = %s", day)
	}
	now := time.Now()
	if err := s.RecordRun(ctx, Run{Kind: "assess", Collection: "austin", Day: "2026-10-01", StartedAt: now, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM runs WHERE collection = 'austin'"); n != 1 {
		t.Error("run collection not recorded")
	}
}

func TestHistory(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	l := sample()
	if err := s.Observe(ctx, "2026-09-28", "", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}
	l.Price.Cents = 260000
	if err := s.Observe(ctx, "2026-09-29", "somerville", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}
	other := sample()
	other.SourceID = "2"
	if err := s.Observe(ctx, "2026-09-30", "somerville", []listing.Listing{l, other}); err != nil {
		t.Fatal(err)
	}

	days, err := s.CollectionDays(ctx, "somerville")
	if err != nil || len(days) != 2 || days[1] != (DayCount{"2026-09-30", 2}) {
		t.Errorf("days = %+v %v", days, err)
	}
	h, err := s.History(ctx, "somerville")
	if err != nil {
		t.Fatal(err)
	}
	if got := h[Key(l)]; len(got) != 3 || got[0] != (Sighting{"2026-09-28", 280000}) || got[2].PriceCents != 260000 {
		t.Errorf("history should include sightings outside the collection: %+v", got)
	}
	if len(h[Key(other)]) != 1 {
		t.Errorf("other history = %+v", h[Key(other)])
	}
}
