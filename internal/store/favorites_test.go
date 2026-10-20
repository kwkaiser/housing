package store

import (
	"errors"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestFavorites(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	l := sample()
	if err := s.Observe(ctx, "2026-09-29", "c", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe(ctx, "2026-09-30", "c", []listing.Listing{l}); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := s.AddFavorite(ctx, l.Source, "missing", at); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("favorite missing listing err = %v", err)
	}
	for range 2 {
		if err := s.AddFavorite(ctx, l.Source, l.SourceID, at); err != nil {
			t.Fatal(err)
		}
	}
	if on, err := s.IsFavorite(ctx, l.Source, l.SourceID); err != nil || !on {
		t.Errorf("IsFavorite = %v %v", on, err)
	}
	if keys, err := s.FavoriteKeys(ctx); err != nil || len(keys) != 1 || !keys[Key(l)] {
		t.Errorf("FavoriteKeys = %v %v", keys, err)
	}
	fs, err := s.Favorites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 || fs[0].Listing.SourceID != l.SourceID || !fs[0].CreatedAt.Equal(at) || fs[0].LastSeen != "2026-09-30" {
		t.Errorf("Favorites = %+v", fs)
	}

	if err := s.RemoveFavorite(ctx, l.Source, l.SourceID); err != nil {
		t.Fatal(err)
	}
	if fs, err := s.Favorites(ctx); err != nil || len(fs) != 0 {
		t.Errorf("after remove = %+v %v", fs, err)
	}
}
