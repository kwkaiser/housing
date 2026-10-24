package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type profileFile struct {
	profile  profile.Profile
	dir      string
	listings map[string]listing.Listing
}

func (s *Service) Import(ctx context.Context, log *slog.Logger) error {
	log = s.logger(log).With("stage", StageImport)
	files, err := readProfiles(ctx, s.cfg.ProfilesDir)
	if err != nil {
		return err
	}
	collections, err := readCollections(s.cfg.CollectionsDir)
	if err != nil {
		return err
	}
	if len(files) == 0 && len(collections) == 0 {
		return fmt.Errorf("nothing to import: no profiles in %s and no collections in %s", s.cfg.ProfilesDir, s.cfg.CollectionsDir)
	}
	db := s.db

	all := make([]profile.Profile, len(files))
	var refCount, copiedCount int
	for i, f := range files {
		p := f.profile
		if len(p.References) == 0 {
			p.References = nil
		}
		all[i] = p
		copied, err := copyMedia(ctx, f.dir, s.images())
		if err != nil {
			return fmt.Errorf("profile %s: %w", p.ID, err)
		}
		refs := make([]listing.Listing, len(p.References))
		for j, r := range p.References {
			l, ok := f.listings[store.Key(listing.Listing{Source: r.Source, SourceID: r.SourceID})]
			if !ok {
				return fmt.Errorf("profile %s: reference %s/%s missing from %s", p.ID, r.Source, r.SourceID, filepath.Join(f.dir, "listings"))
			}
			refs[j] = l
		}
		if err := db.SaveProfile(ctx, p, refs...); err != nil {
			return err
		}
		stored, err := db.Profile(ctx, p.ID)
		if err != nil {
			return err
		}
		if stored.Hash() != p.Hash() {
			return fmt.Errorf("profile %s changed while importing; its assessments would go stale", p.ID)
		}
		refCount += len(refs)
		copiedCount += copied
		log.Info("imported profile", "profile", p.ID, "references", len(refs), "media_copied", copied)
	}
	for _, p := range all {
		if p.IsAvoid() {
			continue
		}
		want, err := profile.Effective(p, all)
		if err != nil {
			return err
		}
		got, err := db.EffectiveProfile(ctx, p.ID)
		if err != nil {
			return err
		}
		if got.Hash() != want.Hash() {
			return fmt.Errorf("profile %s: effective profile changed while importing; its assessments would go stale", p.ID)
		}
	}
	for _, c := range collections {
		if err := db.SaveCollection(ctx, c); err != nil {
			return err
		}
		log.Info("imported collection", "collection", c.ID, "profiles", c.Profiles)
	}
	log.Info("import finished", "profiles", len(files), "references", refCount, "media_copied", copiedCount, "collections", len(collections),
		"db", filepath.Join(s.cfg.DataDir, store.FileName))
	return nil
}

func readProfiles(ctx context.Context, root string) ([]profileFile, error) {
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []profileFile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		path := filepath.Join(dir, "profile.json")
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var p profile.Profile
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if p.ID != e.Name() {
			return nil, fmt.Errorf("%s holds profile %q, expected %q", path, p.ID, e.Name())
		}
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		listings, err := (jsonfile.Persister{}).Load(ctx, filepath.Join(dir, "listings"))
		if err != nil {
			return nil, err
		}
		byKey := make(map[string]listing.Listing, len(listings))
		for _, l := range listings {
			byKey[store.Key(l)] = l
		}
		out = append(out, profileFile{profile: p, dir: dir, listings: byKey})
	}
	return out, nil
}

func readCollections(root string) ([]collection.Collection, error) {
	if root == "" {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []collection.Collection
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var c collection.Collection
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if id := strings.TrimSuffix(filepath.Base(path), ".json"); c.ID != id {
			return nil, fmt.Errorf("%s holds collection %q, expected %q", path, c.ID, id)
		}
		out = append(out, c)
	}
	return out, nil
}

func copyMedia(ctx context.Context, dir string, dst media.DiskStore) (int, error) {
	copied := 0
	for _, sub := range []string{"photos", "collages"} {
		err := filepath.WalkDir(filepath.Join(dir, sub), func(path string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(rel)
			if ok, err := dst.Has(ctx, key); err != nil || ok {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if err := dst.Put(ctx, key, f); err != nil {
				return err
			}
			copied++
			return nil
		})
		if err != nil {
			return copied, err
		}
	}
	return copied, nil
}
