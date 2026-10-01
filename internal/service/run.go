package service

import (
	"cmp"
	"context"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const DefaultCollectionBudgetUSD = 1.0

type RunOptions struct {
	Fetch   FetchOptions
	Collage CollageOptions
	Assess  AssessOptions
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
	}
}

func (s *Service) RunCollection(ctx context.Context, collectionID string, progress Progress) (RunResult, error) {
	c, err := s.Collection(ctx, collectionID)
	if err != nil {
		return RunResult{}, err
	}
	return s.Run(ctx, CollectionRunOptions(c), progress)
}

func (s *Service) Run(ctx context.Context, o RunOptions, progress Progress) (RunResult, error) {
	res := RunResult{Collection: o.Fetch.Collection, Mode: o.Fetch.Mode}
	if _, err := s.cfg.Keys.OpenRouter(); err != nil {
		return res, err
	}
	unlock, err := s.runLock()
	if err != nil {
		return res, err
	}
	defer unlock()

	fetched, err := s.Fetch(ctx, o.Fetch, progress)
	if err != nil {
		return res, err
	}
	res.Day, res.Fetched, res.Distinct, res.Duplicates = fetched.Day, fetched.Observed, fetched.Distinct, fetched.Duplicates
	if len(fetched.Listings) == 0 {
		s.emit(progress, StageRun, "no listings matched; nothing to assess")
		return res, nil
	}

	collaged, err := s.Collage(ctx, o.Collage, fetched.Listings, progress)
	if err != nil {
		return res, err
	}
	res.Collaged, res.Collages, res.WithoutCollages = len(collaged.Listings), collaged.Collages, collaged.WithoutCollages

	assess := o.Assess
	assess.Collection = o.Fetch.Collection
	assess.Mode = o.Fetch.Mode
	assessed, err := s.Assess(ctx, assess, collaged.Listings, progress)
	res.Stats, res.BudgetReached = assessed.Stats, assessed.BudgetReached
	if err != nil {
		if ctx.Err() != nil || res.Stats.Updated == 0 && res.Stats.Failed == 0 {
			return res, err
		}
		res.AssessErr = err
		s.emit(progress, StageRun, "warning: %d listings could not be assessed; continuing to the report\n%v", res.Stats.Failed, err)
	}
	return res, nil
}
