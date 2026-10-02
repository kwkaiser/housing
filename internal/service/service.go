package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/notify"
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
	StageNotify  Stage = "notify"
)

type Clients struct {
	OpenRouter func(apiKey string) openrouter.Completer
	Apify      func(token string, maxChargeUSD float64) apify.Runner
	Provider   func(source listing.Source, runner apify.Runner) (listing.Provider, error)
	Lookup     func(rawURL string, runner apify.Runner) (listing.Lookup, error)
	Photos     media.Fetcher
	Notifier   func(url string) notify.Notifier
}

type Config struct {
	DataDir        string
	ProfilesDir    string
	CollectionsDir string
	Keys           config.Config
	Now            func() time.Time
	Clients        Clients
	Log            *slog.Logger
	Exclusive      bool
}

type Service struct {
	cfg      Config
	db       *store.Store
	unlock   func()
	imported atomic.Bool
}

func Open(ctx context.Context, cfg Config) (*Service, error) {
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
	if c.Notifier == nil {
		c.Notifier = func(url string) notify.Notifier { return notify.NewNtfy(url) }
	}
	s := &Service{cfg: cfg}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	if cfg.Exclusive {
		unlock, err := s.lock()
		if err != nil {
			return nil, err
		}
		s.unlock = unlock
	}
	db, err := store.Open(ctx, filepath.Join(cfg.DataDir, store.FileName))
	if err != nil {
		if s.unlock != nil {
			s.unlock()
		}
		return nil, err
	}
	s.db = db
	return s, nil
}

func (s *Service) Close() error {
	err := s.db.Close()
	if s.unlock != nil {
		s.unlock()
	}
	return err
}

func (s *Service) DataDir() string {
	return s.cfg.DataDir
}

func (s *Service) logger(log *slog.Logger) *slog.Logger {
	if log != nil {
		return log
	}
	if s.cfg.Log != nil {
		return s.cfg.Log
	}
	return slog.New(slog.DiscardHandler)
}

func usd(f float64) float64 {
	return math.Round(f*1e4) / 1e4
}

var ErrNotImported = errors.New("files have not been imported into the database")

func (s *Service) Store() *store.Store {
	return s.db
}

func (s *Service) catalog(ctx context.Context) (*store.Store, error) {
	if s.imported.Load() {
		return s.db, nil
	}
	if err := s.checkImported(ctx, s.db); err != nil {
		return nil, err
	}
	s.imported.Store(true)
	return s.db, nil
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
