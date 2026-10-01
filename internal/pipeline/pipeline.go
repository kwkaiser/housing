package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type Env struct {
	DataDir        string
	ProfilesDir    string
	CollectionsDir string
	Config         config.Config
	Out            io.Writer

	mu sync.Mutex
}

func (e *Env) printf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fmt.Fprintf(e.Out, format, args...)
}

var ErrNotImported = errors.New("listings have not been imported into the database")

func (e *Env) openStore(ctx context.Context) (*store.Store, error) {
	path := filepath.Join(e.DataDir, store.FileName)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		legacy, err := filepath.Glob(filepath.Join(e.DataDir, "*.json"))
		if err != nil {
			return nil, err
		}
		if len(legacy) > 0 {
			return nil, fmt.Errorf("%w: %s holds JSON listings; run `housing import-json` first", ErrNotImported, e.DataDir)
		}
	}
	if err := os.MkdirAll(e.DataDir, 0o755); err != nil {
		return nil, err
	}
	return store.Open(ctx, path)
}

func (e *Env) images() media.DiskStore {
	return media.DiskStore{Root: e.DataDir}
}

func (e *Env) collections() collection.Store {
	return collection.Store{Root: e.CollectionsDir}
}

func (e *Env) profiles() profile.Store {
	return profile.Store{Root: e.ProfilesDir}
}

func noListings(collection string) error {
	if collection != "" {
		return fmt.Errorf("collection %q has no fetched listings yet; run `housing run --collection %s`", collection, collection)
	}
	return fmt.Errorf("no listings have been fetched yet")
}
