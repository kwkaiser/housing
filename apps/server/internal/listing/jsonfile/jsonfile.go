package jsonfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/renameio/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type Persister struct{}

var _ listing.ListingPersister = Persister{}

func (Persister) Persist(ctx context.Context, dir string, listings []listing.Listing) error {
	bySource := map[listing.Source][]listing.Listing{}
	for _, l := range listings {
		bySource[l.Source] = append(bySource[l.Source], l)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for source, batch := range bySource {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dir, string(source)+".json")
		existing, err := readFile(path)
		if err != nil {
			return err
		}
		if err := writeFile(path, upsert(existing, batch)); err != nil {
			return err
		}
	}
	return nil
}

func (Persister) Load(ctx context.Context, dir string) ([]listing.Listing, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)

	var out []listing.Listing
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ls, err := readFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, ls...)
	}
	return out, nil
}

func upsert(existing, batch []listing.Listing) []listing.Listing {
	index := make(map[string]int, len(existing))
	out := slices.Clone(existing)
	for i, l := range out {
		index[l.SourceID] = i
	}
	for _, l := range batch {
		if i, ok := index[l.SourceID]; ok {
			out[i] = l
			continue
		}
		index[l.SourceID] = len(out)
		out = append(out, l)
	}
	slices.SortStableFunc(out, func(a, b listing.Listing) int {
		return strings.Compare(a.SourceID, b.SourceID)
	})
	return out
}

func readFile(path string) ([]listing.Listing, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ls []listing.Listing
	if err := json.Unmarshal(b, &ls); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return ls, nil
}

func writeFile(path string, ls []listing.Listing) error {
	b, err := json.MarshalIndent(ls, "", "  ")
	if err != nil {
		return err
	}
	return renameio.WriteFile(path, append(b, '\n'), 0o644)
}
