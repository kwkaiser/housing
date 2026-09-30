package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
	tmp, err := os.CreateTemp(s.Dir(p.ID), "profile.*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path(p.ID))
}

func (s Store) path(id string) string {
	return filepath.Join(s.Dir(id), "profile.json")
}
