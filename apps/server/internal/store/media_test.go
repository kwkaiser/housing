package store

import (
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestMediaRefs(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	at := func(id string, day time.Time) listing.Listing {
		l := sample()
		l.SourceID, l.ObservedAt = id, day
		l.Photos = []string{"https://example.com/" + id + ".jpg"}
		l.Collages = []string{"collages/" + id + "/0.jpg"}
		return l
	}
	old := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if err := s.Observe(ctx, "2026-09-01", "c", []listing.Listing{at("old", old), at("fav", old)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe(ctx, "2026-10-20", "c", []listing.Listing{at("recent", recent)}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddFavorite(ctx, listing.SourceFacebook, "fav", recent); err != nil {
		t.Fatal(err)
	}
	ref := reference("r1")
	ref.ObservedAt, ref.Photos = old, []string{"https://example.com/r1.jpg"}
	p := wantProfile()
	p.References = p.References[1:]
	if err := s.SaveProfile(ctx, p, ref); err != nil {
		t.Fatal(err)
	}

	refs, err := s.MediaRefs(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	photos, collages := map[string]bool{}, map[string]bool{}
	for _, r := range refs {
		for _, u := range r.Photos {
			photos[u] = true
		}
		for _, c := range r.Collages {
			collages[c] = true
		}
	}
	for _, id := range []string{"recent", "fav", "r1"} {
		if !photos["https://example.com/"+id+".jpg"] || !collages["collages/"+id+"/0.jpg"] {
			t.Errorf("%s media should be kept: photos=%v collages=%v", id, photos, collages)
		}
	}
	if photos["https://example.com/old.jpg"] || collages["collages/old/0.jpg"] {
		t.Errorf("old media should not be kept: photos=%v collages=%v", photos, collages)
	}
}
