package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/dedupe"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	DefaultAssessConcurrency = 4
	DefaultAssessLimit       = 20
	DefaultMaxCostUSD        = 1.0
	checkpointEvery          = 5
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

func (s *Service) Assess(ctx context.Context, o AssessOptions, listings []listing.Listing, progress Progress) (AssessResult, error) {
	var res AssessResult
	started := s.cfg.Now()
	key, err := s.cfg.Keys.OpenRouter()
	if err != nil {
		return res, err
	}
	db, err := s.openCatalog(ctx)
	if err != nil {
		return res, err
	}
	defer db.Close()
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
		s.emit(progress, StageAssess, "assess: %d listings observed on %s", len(listings), day)
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
		assessor := profile.Assessor{Client: client, Model: o.Model, Attempts: profile.DefaultAssessAttempts}

		calibrated, cs, cerr := assessor.AssessListings(ctx, p, refs, t.References,
			func(keys []string) ([][]byte, error) { return profile.ReadCollages(images, keys) },
			profile.BatchOptions{Force: o.Force, Concurrency: 1, Budget: budget,
				Observe: s.assessObserver(progress, "reference ("+p.ID+")", o.Model, 1, budget)},
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
			s.emit(progress, StageAssess, "reference (%s, %s): scores %.1f against its own profile", p.ID, o.Model, ref)
		}

		assessed, st, aerr := assessor.AssessListings(ctx, p, refs, res.Listings,
			func(keys []string) ([][]byte, error) { return profile.ReadCollages(images, keys) },
			profile.BatchOptions{
				Force:           o.Force,
				Limit:           o.Limit,
				Concurrency:     cmpOr(o.Concurrency, DefaultAssessConcurrency),
				MaxCostUSD:      o.MaxCostUSD,
				Budget:          budget,
				Checkpoint:      checkpoint,
				CheckpointEvery: checkpointEvery,
				Observe:         s.assessObserver(progress, "assess ("+p.ID+")", o.Model, cmpOr(o.Concurrency, DefaultAssessConcurrency), budget),
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
		s.emit(progress, StageAssess, "assess (%s, %s): %d model calls ($%.4f), %d listings updated, %d already current, %d failed, %d without collages, %d over limit, %d over budget",
			p.ID, o.Model, st.Calls, st.CostUSD, st.Updated, st.Cached, st.Failed, st.NoCollages, st.OverLimit, st.OverBudget)
	}
	if budget.Exhausted() {
		res.BudgetReached = true
		s.emit(progress, StageAssess, "assess: run budget of $%.2f reached; remaining listings will be graded on the next run", o.MaxRunUSD)
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
	}
	if err != nil {
		run.Error = err.Error()
	}
	if rerr := db.RecordRun(context.WithoutCancel(ctx), run); rerr != nil {
		err = errors.Join(err, rerr)
	}
	return res, err
}

func (s *Service) assessObserver(progress Progress, label, model string, concurrency int, budget *profile.Budget) func(profile.BatchEvent) {
	return func(e profile.BatchEvent) {
		step := fmt.Sprintf("[%d/%d]", e.N, e.Total)
		switch e.Kind {
		case profile.BatchPlanned:
			if e.Total == 0 {
				s.emit(progress, StageAssess, "%s: nothing to grade (%d already current, %d without collages)", label, e.Stats.Cached, e.Stats.NoCollages)
				return
			}
			s.emit(progress, StageAssess, "%s: grading %d listings with %s, %d at a time (%d already current, %d without collages, %d over limit)",
				label, e.Total, model, concurrency, e.Stats.Cached, e.Stats.NoCollages, e.Stats.OverLimit)
		case profile.BatchStarted:
			s.emit(progress, StageAssess, "%s: %s grading %s", label, step, batchListing(e))
		case profile.BatchGraded:
			a := e.Assessment
			s.emit(progress, StageAssess, "%s: %s graded %s: score %.1f, coverage %.0f%%, vibe %d%s · %s · $%.4f (run total $%.4f)",
				label, step, batchListing(e), a.Score, a.Coverage, a.Vibe, gradeFlags(a), e.Elapsed.Round(100*time.Millisecond), a.CostUSD, budget.Spent())
		case profile.BatchFailed:
			s.emit(progress, StageAssess, "%s: %s failed %s after %s: %v", label, step, batchListing(e), e.Elapsed.Round(100*time.Millisecond), e.Err)
		case profile.BatchOverBudget:
			s.emit(progress, StageAssess, "%s: budget reached at $%.4f after %d of %d listings; the rest wait for the next run", label, budget.Spent(), e.N, e.Total)
		}
	}
}

func batchListing(e profile.BatchEvent) string {
	name := e.Listing.Address.Formatted
	if name == "" {
		name = string(e.Listing.Source) + "/" + e.Listing.SourceID
	}
	if e.Units > 1 {
		name += fmt.Sprintf(" (+%d units with the same photos)", e.Units-1)
	}
	return name
}

func gradeFlags(a listing.Assessment) string {
	var out string
	if len(a.Dealbreakers) > 0 {
		out += ", dealbreakers: " + strings.Join(a.Dealbreakers, ", ")
	}
	if len(a.MissingEssentials) > 0 {
		out += ", missing: " + strings.Join(a.MissingEssentials, ", ")
	}
	return out
}
