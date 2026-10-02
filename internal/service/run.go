package service

import (
	"cmp"
	"context"
	"log/slog"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const DefaultCollectionBudgetUSD = 1.0

type RunOptions struct {
	Fetch   FetchOptions
	Collage CollageOptions
	Assess  AssessOptions
	Notify  collection.Notify
}

type RunResult struct {
	Collection      string
	Mode            profile.Mode
	Day             string
	Fetched         int
	Distinct        int
	Duplicates      int
	Collaged        int
	Collages        int
	WithoutCollages int
	Stats           profile.BatchStats
	BudgetReached   bool
	AssessErr       error
	Notified        int
	NotifyErr       error
}

func CollectionRunOptions(c collection.Collection) RunOptions {
	a := DefaultAssessOptions()
	a.Collection = c.ID
	a.ProfileIDs = c.Profiles
	a.Mode = c.Mode
	a.Model = cmp.Or(c.Model, a.Model)
	a.Limit = 0
	a.MaxCostUSD = 0
	a.MaxRunUSD = cmp.Or(c.MaxRunCostUSD, DefaultCollectionBudgetUSD)
	return RunOptions{
		Fetch: FetchOptions{
			Collection:       c.ID,
			Sources:          c.Sources,
			Mode:             c.Mode,
			Search:           c.Search,
			Enrich:           true,
			Photos:           true,
			MaxChargeUSD:     DefaultMaxChargeUSD,
			ApifyConcurrency: apify.DefaultConcurrency,
		},
		Collage: DefaultCollageOptions(),
		Assess:  a,
		Notify:  c.Notify,
	}
}

func (s *Service) RunCollection(ctx context.Context, collectionID string, log *slog.Logger) (RunResult, error) {
	c, err := s.Collection(ctx, collectionID)
	if err != nil {
		return RunResult{}, err
	}
	return s.Run(ctx, CollectionRunOptions(c), log)
}

func (s *Service) Run(ctx context.Context, o RunOptions, log *slog.Logger) (RunResult, error) {
	log = s.logger(log)
	res := RunResult{Collection: o.Fetch.Collection, Mode: o.Fetch.Mode}
	if _, err := s.cfg.Keys.OpenRouter(); err != nil {
		return res, err
	}
	unlock, err := s.runLock()
	if err != nil {
		return res, err
	}
	defer unlock()

	fetched, err := s.Fetch(ctx, o.Fetch, log)
	if err != nil {
		return res, err
	}
	res.Day, res.Fetched, res.Distinct, res.Duplicates = fetched.Day, fetched.Observed, fetched.Distinct, fetched.Duplicates
	if len(fetched.Listings) == 0 {
		log.Info("no listings matched; nothing to assess", "stage", StageRun)
		return res, nil
	}

	collaged, err := s.Collage(ctx, o.Collage, fetched.Listings, log)
	if err != nil {
		return res, err
	}
	res.Collaged, res.Collages, res.WithoutCollages = len(collaged.Listings), collaged.Collages, collaged.WithoutCollages

	assess := o.Assess
	assess.Collection = o.Fetch.Collection
	assess.Mode = o.Fetch.Mode
	assessed, err := s.Assess(ctx, assess, collaged.Listings, log)
	res.Stats, res.BudgetReached = assessed.Stats, assessed.BudgetReached
	if err != nil {
		if ctx.Err() != nil || res.Stats.Updated == 0 && res.Stats.Failed == 0 {
			return res, err
		}
		res.AssessErr = err
		log.Warn("some listings could not be assessed; continuing to the report", "stage", StageRun, "failed", res.Stats.Failed, "err", err)
	}
	res.Notified, res.NotifyErr = s.notify(ctx, o, res.Day, log)
	if res.NotifyErr != nil && ctx.Err() == nil {
		log.Warn("some notifications could not be sent", "stage", StageNotify, "err", res.NotifyErr)
	}
	return res, nil
}
