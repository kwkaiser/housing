package profile

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
)

const DefaultRoot = "profiles"

var ErrNotFound = errors.New("profile not found")

type Store struct {
	Root string
}

func (s Store) Dir(id string) string {
	return filepath.Join(s.Root, id)
}

func (s Store) ListingsDir(id string) string {
	return filepath.Join(s.Dir(id), "listings")
}

func (s Store) Media(id string) media.DiskStore {
	return media.DiskStore{Root: s.Dir(id)}
}

func (s Store) Exists(id string) (bool, error) {
	_, err := os.Stat(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s Store) Load(id string) (Profile, error) {
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return Profile{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return Profile{}, fmt.Errorf("decode %s: %w", s.path(id), err)
	}
	return p, p.Validate()
}

func (s Store) Save(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir(p.ID), 0o755); err != nil {
		return err
	}
	return renameio.WriteFile(s.path(p.ID), append(b, '\n'), 0o644)
}

func (s Store) List() ([]Profile, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ok, err := s.Exists(e.Name())
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		p, err := s.Load(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s Store) Effective(id string) (Profile, error) {
	p, err := s.Load(id)
	if err != nil {
		return Profile{}, err
	}
	if p.IsAvoid() {
		return Profile{}, fmt.Errorf("%q is an avoid profile; it is applied automatically when assessing against a want profile", id)
	}
	all, err := s.List()
	if err != nil {
		return Profile{}, err
	}

	p.Avoid = append([]Criterion{}, p.Avoid...)
	p.References = append([]Reference{}, p.References...)
	for _, ap := range all {
		if !ap.IsAvoid() || len(ap.Avoid) == 0 {
			continue
		}
		for _, c := range ap.Avoid {
			c.ID = ap.ID + "." + c.ID
			p.Avoid = append(p.Avoid, c)
		}
		for _, r := range ap.References {
			r.Profile, r.Avoid = ap.ID, true
			p.References = append(p.References, r)
		}
	}
	return p, p.Validate()
}

func (s Store) References(ctx context.Context, p Profile) ([]ReferenceInput, error) {
	listingsByProfile := map[string]map[string]listing.Listing{}
	var refs []ReferenceInput
	for _, r := range p.References {
		owner := cmp.Or(r.Profile, p.ID)
		byID, ok := listingsByProfile[owner]
		if !ok {
			listings, err := (jsonfile.Persister{}).Load(ctx, s.ListingsDir(owner))
			if err != nil {
				return nil, err
			}
			byID = map[string]listing.Listing{}
			for _, l := range listings {
				byID[string(l.Source)+"/"+l.SourceID] = l
			}
			listingsByProfile[owner] = byID
		}
		l, ok := byID[string(r.Source)+"/"+r.SourceID]
		if !ok {
			return nil, fmt.Errorf("reference %s/%s missing from %s", r.Source, r.SourceID, s.ListingsDir(owner))
		}
		collages, err := ReadCollages(s.Media(owner), r.Collages)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ReferenceInput{Listing: l, Collages: collages, Avoid: r.Avoid})
	}
	return refs, nil
}

func ReadCollages(store media.DiskStore, keys []string) ([][]byte, error) {
	out := make([][]byte, 0, len(keys))
	for _, key := range keys {
		b, err := os.ReadFile(store.Path(key))
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (s Store) path(id string) string {
	return filepath.Join(s.Dir(id), "profile.json")
}
