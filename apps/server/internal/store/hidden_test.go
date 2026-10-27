package store

import (
	"errors"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestHidden(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	l := sample()
	if err := s.Observe(ctx, "2026-09-30", "c", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetHidden(ctx, l.Source, "missing", true); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("hide missing listing err = %v", err)
	}
	if err := s.SetHidden(ctx, l.Source, l.SourceID, true); err != nil {
		t.Fatal(err)
	}
	if on, err := s.IsHidden(ctx, l.Source, l.SourceID); err != nil || !on {
		t.Errorf("IsHidden = %v %v", on, err)
	}
	if err := s.Observe(ctx, "2026-10-01", "c", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}
	if keys, err := s.HiddenKeys(ctx); err != nil || len(keys) != 1 || !keys[Key(l)] {
		t.Errorf("HiddenKeys after observe = %v %v", keys, err)
	}

	if err := s.SetHidden(ctx, l.Source, l.SourceID, false); err != nil {
		t.Fatal(err)
	}
	if keys, err := s.HiddenKeys(ctx); err != nil || len(keys) != 0 {
		t.Errorf("HiddenKeys after unhide = %v %v", keys, err)
	}
}
