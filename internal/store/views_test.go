package store

import (
	"context"
	"errors"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestCurrentListingAndHistory(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	l := sample().WithAssessment("attic", listing.Assessment{Model: "m", Score: 42})
	if err := s.Observe(ctx, "2026-09-29", "c", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}
	l.Price.Cents, l.Description = 270000, "sunnier"
	if err := s.Observe(ctx, "2026-09-30", "c", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}

	got, err := s.CurrentListing(ctx, l.Source, l.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "sunnier" || got.Price.Cents != 270000 {
		t.Errorf("current = %+v", got)
	}
	if a, ok := got.Assessment("attic", "m"); !ok || a.Score != 42 {
		t.Errorf("assessment = %+v %v", a, ok)
	}

	history, err := s.ListingHistory(ctx, l.Source, l.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	want := []Sighting{{"2026-09-29", 280000}, {"2026-09-30", 270000}}
	if len(history) != 2 || history[0] != want[0] || history[1] != want[1] {
		t.Errorf("history = %+v", history)
	}

	if _, err := s.CurrentListing(ctx, l.Source, "missing"); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("missing listing err = %v", err)
	}
}
