package pipeline

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
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
