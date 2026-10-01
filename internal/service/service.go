package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type Stage string

const (
	StageFetch   Stage = "fetch"
	StageDedupe  Stage = "dedupe"
	StagePhotos  Stage = "photos"
	StageCollage Stage = "collage"
	StageAssess  Stage = "assess"
	StageRun     Stage = "run"
	StageProfile Stage = "profile"
	StageImport  Stage = "import"
)

type Event struct {
	Stage   Stage
	Message string
}

type Progress func(Event)

type Clients struct {
	OpenRouter func(apiKey string) openrouter.Completer
	Apify      func(token string, maxChargeUSD float64) apify.Runner
	Provider   func(source listing.Source, runner apify.Runner) (listing.Provider, error)
	Lookup     func(rawURL string, runner apify.Runner) (listing.Lookup, error)
	Photos     media.Fetcher
}

type Config struct {
	DataDir        string
	ProfilesDir    string
	CollectionsDir string
	Keys           config.Config
	Now            func() time.Time
	Clients        Clients
}

type Service struct {
	cfg  Config
	mu   sync.Mutex
	held atomic.Bool
}

func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	c := &cfg.Clients
	if c.OpenRouter == nil {
		c.OpenRouter = func(key string) openrouter.Completer { return openrouter.NewClient(key) }
	}
	if c.Apify == nil {
		c.Apify = func(token string, maxChargeUSD float64) apify.Runner {
			client := apify.NewClient(token)
			client.MaxTotalChargeUSD = maxChargeUSD
			return client
		}
	}
	if c.Provider == nil {
		c.Provider = NewProvider
	}
	if c.Lookup == nil {
		c.Lookup = LookupFor
	}
	return &Service{cfg: cfg}
}

func (s *Service) DataDir() string {
	return s.cfg.DataDir
}

func (s *Service) emit(p Progress, stage Stage, format string, args ...any) {
	if p == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p(Event{Stage: stage, Message: fmt.Sprintf(format, args...)})
}

var ErrNotImported = errors.New("files have not been imported into the database")

func (s *Service) OpenStore(ctx context.Context) (*store.Store, error) {
	return s.openStore(ctx)
}

func (s *Service) openStore(ctx context.Context) (*store.Store, error) {
	dir := s.cfg.DataDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return store.Open(ctx, filepath.Join(dir, store.FileName))
}

func (s *Service) openCatalog(ctx context.Context) (*store.Store, error) {
	db, err := s.openStore(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.checkImported(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Service) checkImported(ctx context.Context, db *store.Store) error {
	profiles, err := db.ProfileCount(ctx)
	if err != nil {
		return err
	}
	if profiles == 0 && s.cfg.ProfilesDir != "" {
		files, err := filepath.Glob(filepath.Join(s.cfg.ProfilesDir, "*", "profile.json"))
		if err != nil {
			return err
		}
		if len(files) > 0 {
			return fmt.Errorf("%w: %s holds profiles; run `housing import` first", ErrNotImported, s.cfg.ProfilesDir)
		}
	}
	collections, err := db.CollectionCount(ctx)
	if err != nil {
		return err
	}
	if collections == 0 && s.cfg.CollectionsDir != "" {
		files, err := filepath.Glob(filepath.Join(s.cfg.CollectionsDir, "*.json"))
		if err != nil {
			return err
		}
		if len(files) > 0 {
			return fmt.Errorf("%w: %s holds collections; run `housing import` first", ErrNotImported, s.cfg.CollectionsDir)
		}
	}
	return nil
}

func (s *Service) photoFetcher() media.Fetcher {
	if s.cfg.Clients.Photos != nil {
		return s.cfg.Clients.Photos
	}
	return media.NewHTTPFetcher()
}

func (s *Service) images() media.DiskStore {
	return media.DiskStore{Root: s.cfg.DataDir}
}

func noListings(collection string) error {
	if collection != "" {
		return fmt.Errorf("collection %q has no fetched listings yet; run it first", collection)
	}
	return fmt.Errorf("no listings have been fetched yet")
}

func cmpOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
