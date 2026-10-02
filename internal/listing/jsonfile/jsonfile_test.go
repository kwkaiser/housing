package jsonfile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func mk(source listing.Source, id string, cents int64) listing.Listing {
	beds := 0
	return listing.Listing{
		Source:     source,
		SourceID:   id,
		URL:        "https://example.com/" + id,
		Offer:      listing.OfferRent,
		Price:      listing.Money{Cents: cents, Currency: "USD"},
		Address:    listing.Address{Formatted: "1 Main St"},
		Beds:       &beds,
		Amenities:  map[listing.Amenity]bool{listing.AmenityDishwasher: true},
		Photos:     []string{},
		ObservedAt: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		Raw:        json.RawMessage(`{"k":1}`),
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	p := Persister{}

	first := []listing.Listing{mk(listing.SourceZillow, "b", 100), mk(listing.SourceZillow, "a", 100), mk(listing.SourceCraigslist, "c", 100)}
	if err := p.Persist(ctx, dir, first); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zillow.json", "craigslist.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	if err := p.Persist(ctx, dir, []listing.Listing{mk(listing.SourceZillow, "a", 200), mk(listing.SourceZillow, "d", 300)}); err != nil {
		t.Fatal(err)
	}

	got, err := p.Load(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	prices := map[string]int64{}
	for _, l := range got {
		ids = append(ids, string(l.Source)+"/"+l.SourceID)
		prices[l.SourceID] = l.Price.Cents
	}
	want := []string{"craigslist/c", "zillow/a", "zillow/b", "zillow/d"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("got %v, want %v", ids, want)
		}
	}
	if prices["a"] != 200 {
		t.Errorf("upsert did not replace a: %d", prices["a"])
	}
	a := got[1]
	var raw bytes.Buffer
	if err := json.Compact(&raw, a.Raw); err != nil {
		t.Fatal(err)
	}
	if a.Beds == nil || *a.Beds != 0 || !a.Amenities[listing.AmenityDishwasher] || raw.String() != `{"k":1}` || !a.ObservedAt.Equal(first[0].ObservedAt) {
		t.Errorf("round trip lost data: %+v", a)
	}
}

func TestLoadMissingDir(t *testing.T) {
	got, err := Persister{}.Load(t.Context(), filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
