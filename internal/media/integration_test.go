//go:build integration

package media

import (
	"context"
	"os"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
)

func TestProcessDataDir(t *testing.T) {
	dir := os.Getenv("HOUSING_DATA_DIR")
	if dir == "" {
		t.Skip("HOUSING_DATA_DIR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	persister := jsonfile.Persister{}
	listings, err := persister.Load(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	p := NewProcessor(NewHTTPFetcher(), DiskStore{Root: dir}, NewGrid())
	processed, err := p.Process(ctx, listings)
	if err != nil {
		t.Fatal(err)
	}

	collages := map[string]bool{}
	without := 0
	for _, l := range processed {
		if len(l.Collages) == 0 && len(l.Photos) > 0 {
			without++
		}
		for _, c := range l.Collages {
			collages[c] = true
		}
	}
	t.Logf("listings=%d distinct collages=%d listings without collages=%d", len(processed), len(collages), without)
	if without > 0 {
		t.Errorf("%d listings with photos got no collages", without)
	}

	if err := persister.Persist(ctx, dir, processed); err != nil {
		t.Fatal(err)
	}
}
