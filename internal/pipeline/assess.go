package pipeline

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const (
	DefaultAssessConcurrency = 4
	checkpointEvery          = 5
)

type AssessOptions struct {
	ProfileID   string
	Model       string
	Mode        profile.Mode
	IDs         []string
	Limit       int
	Concurrency int
	MaxCostUSD  float64
	Force       bool
}

func (e *Env) Assess(ctx context.Context, o AssessOptions, listings []listing.Listing) (profile.Profile, []listing.Listing, profile.BatchStats, error) {
	var stats profile.BatchStats
	key, err := e.Config.OpenRouter()
	if err != nil {
		return profile.Profile{}, nil, stats, err
	}
	p, err := e.profiles().Effective(o.ProfileID)
	if err != nil {
		return profile.Profile{}, nil, stats, err
	}
	refs, err := e.profiles().References(ctx, p)
	if err != nil {
		return p, nil, stats, err
	}

	if listings == nil {
		if listings, err = e.persister().Load(ctx, e.DataDir); err != nil {
			return p, nil, stats, err
		}
	}
	selected := listings
	if o.Mode != "" {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return l.Offer != o.Mode.Offer() })
	}
	if len(o.IDs) > 0 {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return !slices.Contains(o.IDs, l.SourceID) })
		if len(selected) == 0 {
			return p, nil, stats, fmt.Errorf("no stored listings match --id %s", strings.Join(o.IDs, ","))
		}
	}
	if len(selected) == 0 {
		return p, nil, stats, fmt.Errorf("no listings to assess")
	}

	images := e.images()
	checkpoint := func(ls []listing.Listing) error { return e.persister().Persist(ctx, e.DataDir, ls) }
	assessor := profile.Assessor{Client: openrouter.NewClient(key), Model: o.Model, Attempts: profile.DefaultAssessAttempts}
	assessed, stats, assessErr := assessor.AssessListings(ctx, p, refs, selected,
		func(keys []string) ([][]byte, error) { return profile.ReadCollages(images, keys) },
		profile.BatchOptions{
			Force:           o.Force,
			Limit:           o.Limit,
			Concurrency:     cmpOr(o.Concurrency, DefaultAssessConcurrency),
			MaxCostUSD:      o.MaxCostUSD,
			Checkpoint:      checkpoint,
			CheckpointEvery: checkpointEvery,
		},
	)
	if stats.Updated > 0 {
		if err := e.persister().Persist(ctx, e.DataDir, assessed); err != nil {
			return p, assessed, stats, err
		}
	}

	e.printf("assess (%s): %d model calls ($%.4f), %d listings updated, %d already current, %d failed, %d without collages, %d over limit, %d over budget\n",
		o.Model, stats.Calls, stats.CostUSD, stats.Updated, stats.Cached, stats.Failed, stats.NoCollages, stats.OverLimit, stats.OverBudget)
	return p, assessed, stats, assessErr
}
