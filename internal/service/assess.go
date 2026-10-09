package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/dedupe"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

var DefaultAssessConcurrency = runtime.NumCPU()

const (
	DefaultAssessImagePx = 1024
	DefaultAssessLimit   = 20
	DefaultMaxCostUSD    = 1.0
	checkpointEvery      = 5
)

type AssessOptions struct {
	Collection  string
	ProfileIDs  []string
	Model       string
	Mode        profile.Mode
	IDs         []string
	Limit       int
	Concurrency int
	MaxCostUSD  float64
	MaxRunUSD   float64
	Day         string
	Force       bool
}

type AssessResult struct {
	Day           string
	Templates     []report.Template
	Listings      []listing.Listing
	Stats         profile.BatchStats
	BudgetReached bool
}

func DefaultAssessOptions() AssessOptions {
	return AssessOptions{
		Model:       profile.DefaultAssessModel,
		Limit:       DefaultAssessLimit,
		Concurrency: DefaultAssessConcurrency,
		MaxCostUSD:  DefaultMaxCostUSD,
	}
}

func (s *Service) templates(ctx context.Context, db *store.Store, ids []string) ([]template, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one profile is required")
	}
	out := make([]template, len(ids))
	for i, id := range ids {
		p, err := db.EffectiveProfile(ctx, id)
		if err != nil {
			return nil, err
		}
		refs, err := db.ReferenceListings(ctx, p)
		if err != nil {
			return nil, err
		}
		out[i] = template{Template: report.Template{Profile: p, References: own(p, refs)}, refs: refs}
	}
	return out, nil
}

type template struct {
	report.Template
	refs []listing.Listing
}

func own(p profile.Profile, refs []listing.Listing) []listing.Listing {
	var out []listing.Listing
	for i, r := range p.References {
		if r.Profile == "" && !r.Avoid {
			out = append(out, refs[i])
		}
	}
	return out
}

func reportTemplates(ts []template) []report.Template {
	out := make([]report.Template, len(ts))
	for i, t := range ts {
		out[i] = t.Template
	}
	return out
}

func (s *Service) Assess(ctx context.Context, o AssessOptions, listings []listing.Listing, log *slog.Logger) (AssessResult, error) {
	var res AssessResult
	log = s.logger(log).With("stage", StageAssess)
	started := s.cfg.Now()
	key, err := s.cfg.Keys.OpenRouter()
	if err != nil {
		return res, err
	}
	db, err := s.catalog(ctx)
	if err != nil {
		return res, err
	}
	templates, err := s.templates(ctx, db, o.ProfileIDs)
	if err != nil {
		return res, err
	}
	res.Templates = reportTemplates(templates)
	day := cmp.Or(o.Day, store.Day(started))
	switch {
	case listings != nil:
	case len(o.IDs) > 0:
		if listings, err = db.Load(ctx); err != nil {
			return res, err
		}
	default:
		if o.Day == "" {
			if day, err = db.LatestDay(ctx, o.Collection); err != nil {
				return res, err
			}
			if day == "" {
				return res, noListings(o.Collection)
			}
		}
		if listings, err = db.LoadDay(ctx, day, o.Collection); err != nil {
			return res, err
		}
		log.Info("loaded listings", "listings", len(listings), "day", day)
	}
	res.Day = day
	selected := listings
	if o.Mode != "" {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return l.Offer != o.Mode.Offer() })
	}
	if len(o.IDs) == 0 {
		selected = dedupe.Primaries(dedupe.Groups(selected))
	} else {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return !slices.Contains(o.IDs, l.SourceID) })
		if len(selected) == 0 {
			return res, fmt.Errorf("no stored listings match --id %s", strings.Join(o.IDs, ","))
		}
	}
	if len(selected) == 0 {
		return res, fmt.Errorf("no listings to assess")
	}
	res.Listings = selected

	client := s.cfg.Clients.OpenRouter(key)
	images := s.images()
	checkpoint := func(ls []listing.Listing) error { return db.Save(ctx, ls) }
	budget := profile.NewBudget(o.MaxRunUSD)
	var errs []error
	for i, t := range templates {
		if ctx.Err() != nil {
			break
		}
		p := t.Profile
		refs, err := s.references(p, t.refs)
		if err != nil {
			return res, err
		}
		for i := range refs {
			if refs[i].Collages, err = shrinkAll(refs[i].Collages); err != nil {
				return res, err
			}
		}
		assessor := profile.Assessor{Client: client, Model: o.Model, Attempts: profile.DefaultAssessAttempts, ImagePx: DefaultAssessImagePx, ReasoningEffort: profile.DefaultAssessReasoningEffort, MaxTokens: profile.DefaultAssessMaxTokens}

		calibrated, cs, cerr := assessor.AssessListings(ctx, p, refs, t.References,
			readShrunk(images),
			profile.BatchOptions{Force: o.Force, Concurrency: 1, Budget: budget,
				Observe: assessObserver(log.With("profile", p.ID, "model", o.Model, "batch", "reference"), 1, budget)},
		)
		res.Stats.Add(cs)
		if cs.Updated > 0 {
			if err := db.SaveReferenceListings(ctx, p.ID, calibrated); err != nil {
				return res, err
			}
		}
		res.Templates[i].References = calibrated
		if cerr != nil {
			errs = append(errs, fmt.Errorf("%s reference: %w", p.ID, cerr))
		}
		if ref, ok := profile.ReferenceScore(p, calibrated, o.Model); ok {
			log.Info("reference scored against its own profile", "profile", p.ID, "model", o.Model, "score", ref)
		}

		assessed, st, aerr := assessor.AssessListings(ctx, p, refs, res.Listings,
			readShrunk(images),
			profile.BatchOptions{
				Force:           o.Force,
				Limit:           o.Limit,
				Concurrency:     cmpOr(o.Concurrency, DefaultAssessConcurrency),
				MaxCostUSD:      o.MaxCostUSD,
				Budget:          budget,
				Checkpoint:      checkpoint,
				CheckpointEvery: checkpointEvery,
				Observe:         assessObserver(log.With("profile", p.ID, "model", o.Model, "batch", "listings"), cmpOr(o.Concurrency, DefaultAssessConcurrency), budget),
			},
		)
		res.Stats.Add(st)
		res.Listings = assessed
		if st.Updated > 0 {
			if err := db.Save(ctx, assessed); err != nil {
				return res, err
			}
		}
		if aerr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.ID, aerr))
		}
		log.Info("assessed profile", "profile", p.ID, "model", o.Model, "calls", st.Calls, "cost_usd", usd(st.CostUSD), "tokens", st.Tokens.Total(),
			"updated", st.Updated, "cached", st.Cached, "failed", st.Failed, "no_collages", st.NoCollages, "over_limit", st.OverLimit, "over_budget", st.OverBudget)
	}
	if budget.Exhausted() {
		res.BudgetReached = true
		log.Info("run budget reached; remaining listings will be graded on the next run", "max_run_usd", o.MaxRunUSD)
	}

	err = errors.Join(errs...)
	run := store.Run{
		Kind:       "assess",
		Collection: o.Collection,
		Day:        day,
		StartedAt:  started,
		FinishedAt: s.cfg.Now(),
		Model:      o.Model,
		Profiles:   o.ProfileIDs,
		Calls:      res.Stats.Calls,
		CostUSD:    res.Stats.CostUSD,
		Failed:     res.Stats.Failed,
		OverBudget: res.Stats.OverBudget,
		Tokens:     res.Stats.Tokens,
	}
	if err != nil {
		run.Error = err.Error()
	}
	if rerr := db.RecordRun(context.WithoutCancel(ctx), run); rerr != nil {
		err = errors.Join(err, rerr)
	}
	return res, err
}

func assessObserver(log *slog.Logger, concurrency int, budget *profile.Budget) func(profile.BatchEvent) {
	return func(e profile.BatchEvent) {
		step := fmt.Sprintf("%d/%d", e.N, e.Total)
		switch e.Kind {
		case profile.BatchPlanned:
			if e.Total == 0 {
				log.Info("nothing to grade", "cached", e.Stats.Cached, "no_collages", e.Stats.NoCollages)
				return
			}
			log.Info("grading listings", "listings", e.Total, "concurrency", concurrency,
				"cached", e.Stats.Cached, "no_collages", e.Stats.NoCollages, "over_limit", e.Stats.OverLimit)
		case profile.BatchStarted:
			log.Info("grading", append([]any{"step", step}, listingAttrs(e)...)...)
		case profile.BatchGraded:
			a := e.Assessment
			attrs := append([]any{"step", step}, listingAttrs(e)...)
			attrs = append(attrs, "score", a.Score, "coverage", a.Coverage)
			if len(a.Dealbreakers) > 0 {
				attrs = append(attrs, "dealbreakers", a.Dealbreakers)
			}
			if len(a.MissingEssentials) > 0 {
				attrs = append(attrs, "missing", a.MissingEssentials)
			}
			attrs = append(attrs, "elapsed", e.Elapsed.Round(100*time.Millisecond).String(), "tokens", a.Tokens.Total(),
				"cost_usd", usd(a.CostUSD), "run_cost_usd", usd(budget.Spent()), "run_tokens", budget.Tokens().Total())
			log.Info("graded", attrs...)
		case profile.BatchFailed:
			attrs := append([]any{"step", step}, listingAttrs(e)...)
			attrs = append(attrs, "elapsed", e.Elapsed.Round(100*time.Millisecond).String(), "tokens", e.Assessment.Tokens.Total(),
				"cost_usd", usd(e.Assessment.CostUSD), "err", e.Err)
			log.Warn("grading failed", attrs...)
		case profile.BatchOverBudget:
			log.Info("budget reached; the rest wait for the next run", "spent_usd", usd(budget.Spent()), "started", e.N, "total", e.Total)
		}
	}
}

func listingAttrs(e profile.BatchEvent) []any {
	name := e.Listing.Address.Formatted
	if name == "" {
		name = string(e.Listing.Source) + "/" + e.Listing.SourceID
	}
	attrs := []any{"listing", name}
	if e.Units > 1 {
		attrs = append(attrs, "same_photo_units", e.Units-1)
	}
	return attrs
}

func readShrunk(images media.DiskStore) func([]string) ([][]byte, error) {
	return func(keys []string) ([][]byte, error) {
		collages, err := profile.ReadCollages(images, keys)
		if err != nil {
			return nil, err
		}
		return shrinkAll(collages)
	}
}

func shrinkAll(images [][]byte) ([][]byte, error) {
	out := make([][]byte, len(images))
	for i, b := range images {
		small, err := media.Shrink(b, DefaultAssessImagePx)
		if err != nil {
			return nil, err
		}
		out[i] = small
	}
	return out, nil
}
