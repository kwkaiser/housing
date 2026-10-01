package collection

import (
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
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const DefaultRoot = "collections"

var ErrNotFound = errors.New("collection not found")

type Collection struct {
	ID            string           `json:"id"`
	Mode          profile.Mode     `json:"mode"`
	Sources       []listing.Source `json:"sources"`
	Profiles      []string         `json:"profiles"`
	Search        profile.Search   `json:"search"`
	Model         string           `json:"model,omitempty"`
	MaxRunCostUSD float64          `json:"max_run_cost_usd,omitempty"`
}

func (c Collection) Validate() error {
	var errs []error
	if err := profile.ValidID(c.ID); err != nil {
		errs = append(errs, err)
	}
	if _, err := profile.ParseMode(string(c.Mode)); err != nil {
		errs = append(errs, err)
	}
	if len(c.Sources) == 0 {
		errs = append(errs, fmt.Errorf("at least one source is required"))
	}
	if len(c.Profiles) == 0 {
		errs = append(errs, fmt.Errorf("at least one profile is required"))
	}
	if c.Search.Location == "" {
		errs = append(errs, fmt.Errorf("a search location is required"))
	}
	if c.MaxRunCostUSD < 0 {
		errs = append(errs, fmt.Errorf("max run cost must not be negative"))
	}
	if err := c.Search.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("collection %q: %w", c.ID, err)
	}
	return nil
}

type Store struct {
	Root string
}

func (s Store) Load(id string) (Collection, error) {
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return Collection{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Collection{}, err
	}
	var c Collection
	if err := json.Unmarshal(b, &c); err != nil {
		return Collection{}, fmt.Errorf("decode %s: %w", s.path(id), err)
	}
	if c.ID != id {
		return Collection{}, fmt.Errorf("%s holds collection %q, expected %q", s.path(id), c.ID, id)
	}
	return c, c.Validate()
}

func (s Store) Save(c Collection) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	return renameio.WriteFile(s.path(c.ID), append(b, '\n'), 0o644)
}

func (s Store) Exists(id string) (bool, error) {
	_, err := os.Stat(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s Store) List() ([]Collection, error) {
	paths, err := filepath.Glob(filepath.Join(s.Root, "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []Collection
	for _, p := range paths {
		c, err := s.Load(strings.TrimSuffix(filepath.Base(p), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (s Store) path(id string) string {
	return filepath.Join(s.Root, id+".json")
}
