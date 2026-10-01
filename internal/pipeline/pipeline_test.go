package pipeline

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func TestSplitAmenities(t *testing.T) {
	filterable, deferred := SplitAmenities(
		[]listing.Amenity{listing.AmenityDishwasher, listing.AmenityParking},
		[]listing.Amenity{listing.AmenityParking},
	)
	if len(filterable) != 1 || filterable[0] != listing.AmenityParking || len(deferred) != 1 || deferred[0] != listing.AmenityDishwasher {
		t.Errorf("filterable=%v deferred=%v", filterable, deferred)
	}
}

func TestFetchValidates(t *testing.T) {
	env := &Env{DataDir: t.TempDir(), Out: &bytes.Buffer{}, Config: config.Config{ApifyToken: "x"}}
	cases := map[string]FetchOptions{
		"location":           {Sources: []listing.Source{listing.SourceZillow}, Mode: profile.ModeRent, Search: profile.Search{Limit: 1}},
		"limit":              {Sources: []listing.Source{listing.SourceZillow}, Mode: profile.ModeRent, Search: profile.Search{Location: "x"}},
		"one source":         {Mode: profile.ModeRent, Search: profile.Search{Location: "x", Limit: 1}},
		"unsupported source": {Sources: []listing.Source{"myspace"}, Mode: profile.ModeRent, Search: profile.Search{Location: "x", Limit: 1}},
		"enrich": {Sources: []listing.Source{listing.SourceZillow}, Mode: profile.ModeBuy,
			Search: profile.Search{Location: "x", Limit: 1, Amenities: []listing.Amenity{listing.AmenityDishwasher}}},
	}
	for want, o := range cases {
		if _, err := env.Fetch(context.Background(), o); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
}

func TestSavedSearch(t *testing.T) {
	dir := t.TempDir()
	env := &Env{ProfilesDir: dir}
	if s, err := env.SavedSearch("", profile.ModeRent); err != nil || s.Limit != DefaultFetchLimit {
		t.Fatalf("got %+v %v", s, err)
	}
	p := profile.Profile{ID: "attic", Ignore: profile.DefaultIgnore, Searches: map[profile.Mode]profile.Search{profile.ModeBuy: {Location: "Somerville, MA"}}}
	if err := (profile.Store{Root: dir}).Save(p); err != nil {
		t.Fatal(err)
	}
	if s, err := env.SavedSearch("attic", profile.ModeBuy); err != nil || s.Location != "Somerville, MA" || s.Limit != DefaultFetchLimit {
		t.Errorf("got %+v %v", s, err)
	}
	if _, err := env.SavedSearch("attic", profile.ModeRent); err == nil {
		t.Error("missing saved search should fail")
	}
}

func TestCarryOver(t *testing.T) {
	prev := listing.Listing{Source: listing.SourceZillow, SourceID: "1", Collages: []string{"c/0.jpg"}}
	prev = prev.WithAssessment("attic", listing.Assessment{Model: "m", Score: 42})
	fresh := []listing.Listing{
		{Source: listing.SourceZillow, SourceID: "1", Description: "updated"},
		{Source: listing.SourceRedfin, SourceID: "1"},
	}
	got := CarryOver([]listing.Listing{prev}, fresh)
	if a, ok := got[0].Assessment("attic", "m"); !ok || a.Score != 42 || got[0].Collages[0] != "c/0.jpg" || got[0].Description != "updated" {
		t.Errorf("re-fetched listing should keep prior assessments and collages: %+v", got[0])
	}
	if got[1].Assessments != nil {
		t.Error("carry over must match on source as well as id")
	}
}

func TestFreshGroups(t *testing.T) {
	beds := 2
	addr := listing.Address{Street: "23 Everett St", PostalCode: "02138"}
	l := func(source listing.Source, id string) listing.Listing {
		return listing.Listing{Source: source, SourceID: id, Offer: listing.OfferRent, Beds: &beds, Address: addr, Price: listing.Money{Cents: 280000, Currency: "USD"}}
	}
	stored := []listing.Listing{
		l(listing.SourceZillow, "z").WithAssessment("attic", listing.Assessment{Model: "m"}),
		{Source: listing.SourceZillow, SourceID: "old", Offer: listing.OfferRent, Address: listing.Address{Street: "1 Elm St"}},
	}
	fresh := []listing.Listing{l(listing.SourceFacebook, "f"), l(listing.SourceRedfin, "r")}

	got := FreshGroups(stored, fresh)
	if len(got) != 1 || got[0].Primary.SourceID != "z" || len(got[0].Others) != 2 {
		t.Errorf("fresh duplicates of an assessed listing should collapse onto it: %+v", got)
	}
}

func TestImportJSON(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env := &Env{DataDir: dir, Out: &bytes.Buffer{}}
	ls := []listing.Listing{
		{Source: listing.SourceZillow, SourceID: "1", Offer: listing.OfferRent, ObservedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)},
		{Source: listing.SourceRedfin, SourceID: "2", Offer: listing.OfferRent, ObservedAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)},
	}
	if err := (jsonfile.Persister{}).Persist(ctx, dir, ls); err != nil {
		t.Fatal(err)
	}
	if _, err := env.openStore(ctx); !errors.Is(err, ErrNotImported) {
		t.Fatalf("unimported JSON should be reported: %v", err)
	}
	if err := env.ImportJSON(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := env.openStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.Load(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %d listings, %v", len(got), err)
	}
}

func TestLock(t *testing.T) {
	env := &Env{DataDir: t.TempDir()}
	unlock, err := env.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Lock(); !errors.Is(err, ErrLocked) {
		t.Errorf("second lock should fail: %v", err)
	}
	unlock()
	again, err := env.Lock()
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	again()
}
