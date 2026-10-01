package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/dedupe"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

const (
	DefaultAssessConcurrency = 4
	checkpointEvery          = 5
)

type AssessOptions struct {
	ProfileIDs  []string
	Model       string
	Mode        profile.Mode
	IDs         []string
	Limit       int
	Concurrency int
	MaxCostUSD  float64
	Force       bool
}

func (e *Env) templates(ctx context.Context, ids []string) ([]report.Template, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one profile is required")
	}
	out := make([]report.Template, len(ids))
	for i, id := range ids {
		p, err := e.profiles().Effective(id)
		if err != nil {
			return nil, err
		}
		refs, err := e.profiles().ReferenceListings(ctx, p)
		if err != nil {
			return nil, err
		}
		out[i] = report.Template{Profile: p, References: refs}
	}
	return out, nil
}

func (e *Env) Assess(ctx context.Context, o AssessOptions, listings []listing.Listing) ([]report.Template, []listing.Listing, profile.BatchStats, error) {
	var stats profile.BatchStats
	key, err := e.Config.OpenRouter()
	if err != nil {
		return nil, nil, stats, err
	}
	templates, err := e.templates(ctx, o.ProfileIDs)
	if err != nil {
		return nil, nil, stats, err
	}

	if listings == nil {
		if listings, err = e.persister().Load(ctx, e.DataDir); err != nil {
			return templates, nil, stats, err
		}
	}
	selected := listings
	if o.Mode != "" {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return l.Offer != o.Mode.Offer() })
	}
	if len(o.IDs) == 0 {
		selected = dedupe.Primaries(dedupe.Groups(selected))
	} else {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return !slices.Contains(o.IDs, l.SourceID) })
		if len(selected) == 0 {
			return templates, nil, stats, fmt.Errorf("no stored listings match --id %s", strings.Join(o.IDs, ","))
		}
	}
	if len(selected) == 0 {
		return templates, nil, stats, fmt.Errorf("no listings to assess")
	}

	client := openrouter.NewClient(key)
	images := e.images()
	checkpoint := func(ls []listing.Listing) error { return e.persister().Persist(ctx, e.DataDir, ls) }
	var errs []error
	for i, t := range templates {
		if ctx.Err() != nil {
			break
		}
		p := t.Profile
		refs, err := e.profiles().References(ctx, p)
		if err != nil {
			return templates, selected, stats, err
		}
		assessor := profile.Assessor{Client: client, Model: o.Model, Attempts: profile.DefaultAssessAttempts}

		calibrated, cs, cerr := assessor.AssessListings(ctx, p, refs, t.References,
			func(keys []string) ([][]byte, error) { return profile.ReadCollages(e.profiles().Media(p.ID), keys) },
			profile.BatchOptions{Force: o.Force, Concurrency: 1},
		)
		stats.Add(cs)
		if cs.Updated > 0 {
			if err := e.profiles().SaveReferenceListings(ctx, p.ID, calibrated); err != nil {
				return templates, selected, stats, err
			}
		}
		templates[i].References = calibrated
		if cerr != nil {
			errs = append(errs, fmt.Errorf("%s reference: %w", p.ID, cerr))
		}
		if ref, ok := profile.ReferenceScore(p, calibrated, o.Model); ok {
			e.printf("reference (%s, %s): scores %.1f against its own profile\n", p.ID, o.Model, ref)
		}

		assessed, s, aerr := assessor.AssessListings(ctx, p, refs, selected,
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
		stats.Add(s)
		selected = assessed
		if s.Updated > 0 {
			if err := e.persister().Persist(ctx, e.DataDir, assessed); err != nil {
				return templates, selected, stats, err
			}
		}
		if aerr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.ID, aerr))
		}
		e.printf("assess (%s, %s): %d model calls ($%.4f), %d listings updated, %d already current, %d failed, %d without collages, %d over limit, %d over budget\n",
			p.ID, o.Model, s.Calls, s.CostUSD, s.Updated, s.Cached, s.Failed, s.NoCollages, s.OverLimit, s.OverBudget)
	}
	return templates, selected, stats, errors.Join(errs...)
}
