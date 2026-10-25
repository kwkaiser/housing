package service

import (
	"bytes"
	"os"
	"slices"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
)

func TestPruneMedia(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	svc := openService(t, Config{DataDir: dir, Now: func() time.Time { return now }})
	images := media.DiskStore{Root: dir}

	at := func(id string, seen time.Time) listing.Listing {
		return listing.Listing{
			Source: listing.SourceZillow, SourceID: id, URL: "https://zillow.com/" + id, Offer: listing.OfferRent,
			Price: listing.Money{Cents: 100000, Currency: "USD"}, Address: listing.Address{Formatted: id},
			Photos: []string{"https://example.com/" + id + ".jpg"}, Collages: []string{"collages/" + id[:2] + "/" + id + "/0.jpg"},
			ObservedAt: seen,
		}
	}
	old, recent := now.AddDate(0, 0, -45), now.AddDate(0, 0, -3)
	ls := []listing.Listing{at("stale", old), at("favorite", old), at("current", recent)}
	if err := svc.Store().Observe(ctx, "2026-09-10", "c", ls[:2]); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store().Observe(ctx, "2026-10-22", "c", ls[2:]); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store().AddFavorite(ctx, listing.SourceZillow, "favorite", recent); err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		for _, key := range []string{media.PhotoKey(l.Photos[0]), l.Collages[0]} {
			if err := images.Put(ctx, key, bytes.NewReader([]byte("img"))); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(images.Path(key), old, old); err != nil {
				t.Fatal(err)
			}
		}
	}

	res, err := svc.PruneMedia(ctx, PruneOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 2 || res.Kept != 4 || !res.Since.Equal(now.Add(-DefaultMediaRetention)) {
		t.Errorf("prune = %+v", res)
	}
	for _, l := range ls {
		has, _ := images.Has(ctx, media.PhotoKey(l.Photos[0]))
		stored := svc.StoredCollages(ctx, l.Collages)
		if want := l.SourceID != "stale"; has != want || slices.Equal(stored, l.Collages) != want {
			t.Errorf("%s: photo kept = %v, collages = %v", l.SourceID, has, stored)
		}
	}
}
