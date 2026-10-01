package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func reference(id string) listing.Listing {
	l := sample()
	l.Source, l.SourceID, l.URL = listing.SourceZillow, id, "https://zillow.com/"+id
	l.Collages = []string{"collages/" + id + "/0.jpg"}
	return l
}

func wantProfile() profile.Profile {
	return profile.Profile{
		ID:      "attic",
		Name:    "Attic",
		Summary: "Sunny attic",
		Notes:   []string{"skylights"},
		Want: []profile.Criterion{
			{ID: "skylights", Label: "Skylights", LookFor: "roof windows", Keywords: []string{"skylight"}, Importance: profile.Essential, Evidence: profile.EvidenceEither},
		},
		Avoid:  []profile.Criterion{{ID: "basement", Label: "Basement", Importance: profile.High, Evidence: profile.EvidencePhotos}},
		Ignore: profile.DefaultIgnore,
		References: []profile.Reference{
			{Source: listing.SourceZillow, SourceID: "r2", URL: "https://zillow.com/r2", Collages: []string{"collages/r2/0.jpg"}},
			{Source: listing.SourceZillow, SourceID: "r1", URL: "https://zillow.com/r1", Collages: []string{"collages/r1/0.jpg", "collages/r1/1.jpg"}},
		},
		Searches: map[profile.Mode]profile.Search{profile.ModeRent: {Location: "Somerville, MA", Limit: 5}},
		Drafted:  &profile.Drafted{Model: "m", At: time.Date(2026, 9, 30, 12, 0, 0, 123, time.UTC), CostUSD: 0.01},
	}
}

func corporate() profile.Profile {
	return profile.Profile{
		ID:         "corporate",
		Kind:       profile.KindAvoid,
		Avoid:      []profile.Criterion{{ID: "large_building", Label: "Large building", Importance: profile.Essential, Evidence: profile.EvidenceEither}},
		Ignore:     profile.DefaultIgnore,
		References: []profile.Reference{{Source: listing.SourceZillow, SourceID: "c1", URL: "https://zillow.com/c1", Collages: []string{"collages/c1/0.jpg"}}},
	}
}

func TestProfileRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p := wantProfile()
	if err := s.SaveProfile(ctx, p, reference("r1")); err == nil {
		t.Error("a reference without a stored listing should be rejected")
	}
	if err := s.SaveProfile(ctx, p, reference("r1"), reference("r2")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Profile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash() != p.Hash() {
		t.Error("the profile hash must survive a round trip through the database")
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("got %+v\nwant %+v", got, p)
	}
	if err := s.SaveProfile(ctx, got); err != nil {
		t.Fatalf("resaving should reuse the stored references: %v", err)
	}
	if _, err := s.Profile(ctx, "missing"); !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("got %v", err)
	}
	bare := profile.Profile{ID: "bare", Ignore: []string{}}
	if err := s.SaveProfile(ctx, bare); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Profile(ctx, "bare"); got.Hash() != bare.Hash() || got.Want != nil || got.Ignore == nil {
		t.Errorf("nil and empty criteria should be kept apart: %+v", got)
	}
	if n, _ := s.ProfileCount(ctx); n != 2 {
		t.Errorf("count = %d", n)
	}
	if ok, _ := s.ProfileExists(ctx, "attic"); !ok {
		t.Error("attic should exist")
	}
	if listings, _ := s.Load(ctx); len(listings) != 0 {
		t.Errorf("reference listings are never observed and should not load as listings: %+v", listings)
	}
}

func TestEffectiveProfile(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	want, ap := wantProfile(), corporate()
	if err := s.SaveProfile(ctx, want, reference("r1"), reference("r2")); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProfile(ctx, ap, reference("c1")); err != nil {
		t.Fatal(err)
	}
	eff, err := s.EffectiveProfile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := profile.Effective(want, []profile.Profile{want, ap})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eff, expected) || eff.Hash() != expected.Hash() {
		t.Errorf("got %+v\nwant %+v", eff, expected)
	}
	if _, err := s.EffectiveProfile(ctx, "corporate"); err == nil {
		t.Error("an avoid profile has no effective form")
	}
	if err := s.SaveProfile(ctx, eff); err == nil {
		t.Error("saving an effective profile should be rejected")
	}

	refs, err := s.ReferenceListings(ctx, eff)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 || refs[0].SourceID != "r2" || refs[1].SourceID != "r1" || refs[2].SourceID != "c1" || refs[1].Collages[0] != "collages/r1/0.jpg" {
		t.Errorf("references = %+v", refs)
	}
}

func TestReferenceListingsArePinned(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p := wantProfile()
	p.References = p.References[1:]
	ref := reference("r1")
	if err := s.SaveProfile(ctx, p, ref); err != nil {
		t.Fatal(err)
	}

	observed := ref
	observed.Description = "fetched later"
	observed.Collages = []string{"collages/other/0.jpg"}
	if err := s.Observe(ctx, "2026-09-30", "", []listing.Listing{observed}); err != nil {
		t.Fatal(err)
	}
	refs, err := s.ReferenceListings(ctx, p)
	if err != nil || len(refs) != 1 || refs[0].Description != "sunny" || refs[0].Collages[0] != "collages/r1/0.jpg" {
		t.Fatalf("the reference should keep the version it was saved with: %+v %v", refs, err)
	}

	graded := refs[0].WithAssessment("attic", listing.Assessment{Model: "m", ProfileHash: p.Hash(), InputHash: profile.InputHash(refs[0]), Score: 80})
	if err := s.SaveReferenceListings(ctx, "attic", []listing.Listing{graded}); err != nil {
		t.Fatal(err)
	}
	refs, _ = s.ReferenceListings(ctx, p)
	if score, ok := profile.ReferenceScore(p, refs, "m"); !ok || score != 80 {
		t.Errorf("calibration should load back: %v %v", score, ok)
	}
	if err := s.SaveReferenceListings(ctx, "attic", []listing.Listing{reference("nope")}); err == nil {
		t.Error("saving a listing that is not a reference should fail")
	}
}

func TestCollectionRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	maxPrice := 4000
	c := collection.Collection{
		ID: "somerville", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow, listing.SourceRedfin},
		Profiles: []string{"zeta", "attic"}, Search: profile.Search{Location: "02144", MaxPrice: &maxPrice, Limit: 50},
		Model: "m", MaxRunCostUSD: 1,
	}
	if err := s.SaveCollection(ctx, c); !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("unknown profiles should be rejected: %v", err)
	}
	for _, id := range c.Profiles {
		if err := s.SaveProfile(ctx, profile.Profile{ID: id, Ignore: profile.DefaultIgnore}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := s.Collection(ctx, "somerville")
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("got %+v %v", got, err)
	}
	c.Profiles = []string{"attic"}
	if err := s.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if all, err := s.Collections(ctx); err != nil || len(all) != 1 || !reflect.DeepEqual(all[0].Profiles, []string{"attic"}) {
		t.Errorf("list = %+v %v", all, err)
	}
	if _, err := s.Collection(ctx, "missing"); !errors.Is(err, collection.ErrNotFound) {
		t.Errorf("got %v", err)
	}
	if n, _ := s.CollectionCount(ctx); n != 1 {
		t.Errorf("count = %d", n)
	}
}
