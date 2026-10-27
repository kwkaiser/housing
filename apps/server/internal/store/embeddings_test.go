package store

import (
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func withCollages(id string, collages ...string) listing.Listing {
	l := sample()
	l.SourceID = id
	l.Collages = collages
	return l
}

func pendingIDs(t *testing.T, s *Store, f PendingFilter) map[string]EmbedTarget {
	t.Helper()
	ts, err := s.PendingEmbeddings(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]EmbedTarget{}
	for _, tg := range ts {
		out[tg.Listing.SourceID] = tg
	}
	return out
}

func TestPendingEmbeddings(t *testing.T) {
	s := open(t)
	ctx := t.Context()
	if err := s.Save(ctx, []listing.Listing{withCollages("1", "c1"), withCollages("2", "c2"), withCollages("3")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Observe(ctx, "2026-10-01", "boston", []listing.Listing{withCollages("1", "c1")}); err != nil {
		t.Fatal(err)
	}

	got := pendingIDs(t, s, PendingFilter{Model: "m"})
	if len(got) != 2 || got["1"].Listing.Collages[0] != "c1" || got["1"].Listing.Description != "sunny" {
		t.Fatalf("pending = %+v, want listings 1 and 2 with their collages", got)
	}
	if got := pendingIDs(t, s, PendingFilter{Model: "m", Collection: "boston"}); len(got) != 1 || got["1"].VersionID == 0 {
		t.Errorf("collection pending = %+v, want only listing 1", got)
	}
	if got := pendingIDs(t, s, PendingFilter{Model: "m", Limit: 1}); len(got) != 1 {
		t.Errorf("limited pending = %d, want 1", len(got))
	}

	one := got["1"]
	if err := s.SaveEmbedding(ctx, Embedding{VersionID: one.VersionID, Model: "m", InputHash: one.InputHash, Vector: []float32{1, 0}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if got := pendingIDs(t, s, PendingFilter{Model: "m"}); len(got) != 1 || got["2"].VersionID == 0 {
		t.Errorf("pending after embedding = %+v, want only listing 2", got)
	}
	if got := pendingIDs(t, s, PendingFilter{Model: "other"}); len(got) != 2 {
		t.Errorf("another model should still need both listings, got %d", len(got))
	}

	if err := s.Save(ctx, []listing.Listing{withCollages("1", "c1", "c1b")}); err != nil {
		t.Fatal(err)
	}
	if got := pendingIDs(t, s, PendingFilter{Model: "m"}); len(got) != 2 {
		t.Errorf("new collages on the same version should need a new embedding, got %+v", got)
	}
}

func TestNearest(t *testing.T) {
	s := open(t)
	ctx := t.Context()
	if err := s.Save(ctx, []listing.Listing{withCollages("1", "c1"), withCollages("2", "c2"), withCollages("3", "c3")}); err != nil {
		t.Fatal(err)
	}
	vectors := map[string][]float32{"1": {1, 0, 0}, "2": {0.9, 0.1, 0}, "3": {0, 0, 1}}
	for id, tg := range pendingIDs(t, s, PendingFilter{Model: "m"}) {
		if err := s.SaveEmbedding(ctx, Embedding{VersionID: tg.VersionID, Model: "m", InputHash: tg.InputHash, Vector: vectors[id], CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	ns, err := s.Nearest(ctx, "m", []float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 2 || ns[0].SourceID != "1" || ns[1].SourceID != "2" || ns[0].Source != listing.SourceFacebook {
		t.Fatalf("nearest = %+v, want listings 1 then 2", ns)
	}
	if ns[0].Distance > 1e-6 || ns[1].Distance <= ns[0].Distance {
		t.Errorf("distances = %v, %v", ns[0].Distance, ns[1].Distance)
	}

	tg := pendingIDs(t, s, PendingFilter{Model: "n"})["3"]
	if err := s.SaveEmbedding(ctx, Embedding{VersionID: tg.VersionID, Model: "m", InputHash: tg.InputHash, Vector: []float32{1, 0, 0}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `SELECT count(*) FROM embeddings`); n != 3 {
		t.Errorf("re-embedding should replace the row, got %d rows", n)
	}
	if n := count(t, s, `SELECT count(*) FROM embedding_vectors`); n != 3 {
		t.Errorf("re-embedding should replace the vector, got %d vectors", n)
	}
	if ns, _ := s.Nearest(ctx, "m", []float32{1, 0, 0}, 3); len(ns) != 3 || ns[2].SourceID != "2" {
		t.Errorf("nearest after replacing listing 3 = %+v", ns)
	}

	if ns, err := s.Nearest(ctx, "other", []float32{1, 0, 0}, 3); err != nil || len(ns) != 0 {
		t.Errorf("nearest for another model = %+v %v, want none", ns, err)
	}
	if err := s.SaveEmbedding(ctx, Embedding{VersionID: tg.VersionID, Model: "wide", InputHash: tg.InputHash, Vector: []float32{1, 0}, CreatedAt: time.Now()}); err == nil {
		t.Error("a vector with different dimensions should be refused")
	}
}
