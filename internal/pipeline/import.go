package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func (e *Env) ImportJSON(ctx context.Context) error {
	listings, err := (jsonfile.Persister{}).Load(ctx, e.DataDir)
	if err != nil {
		return err
	}
	if len(listings) == 0 {
		return fmt.Errorf("no JSON listings found in %s", e.DataDir)
	}
	byDay := map[string][]listing.Listing{}
	for _, l := range listings {
		day := store.Day(l.ObservedAt)
		byDay[day] = append(byDay[day], l)
	}

	path := filepath.Join(e.DataDir, store.FileName)
	db, err := store.Open(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	slices.Sort(days)
	for _, day := range days {
		if err := db.Observe(ctx, day, "", byDay[day]); err != nil {
			return err
		}
		e.printf("import: %d listings observed on %s\n", len(byDay[day]), day)
	}
	e.printf("import: %d listings written to %s; the JSON files are no longer read and can be removed\n", len(listings), path)
	return nil
}
