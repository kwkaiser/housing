package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
	"git.kwkaiser.io/kwkaiser/housing/internal/telemetry"
)

const (
	DefaultEmbedModel       = "google/gemini-embedding-2"
	DefaultEmbedDimensions  = 768
	DefaultEmbedConcurrency = 4
	DefaultEmbedMaxCostUSD  = 1.0
)

type EmbedOptions struct {
	Collection  string
	Model       string
	Dimensions  int
	Limit       int
	Concurrency int
	MaxCostUSD  float64
	DryRun      bool
}

type EmbedResult struct {
	Pending       int
	Collages      int
	Embedded      int
	Failed        int
	Tokens        int64
	CostUSD       float64
	BudgetReached bool
}

func DefaultEmbedOptions() EmbedOptions {
	return EmbedOptions{
		Model:       DefaultEmbedModel,
		Dimensions:  DefaultEmbedDimensions,
		Concurrency: DefaultEmbedConcurrency,
		MaxCostUSD:  DefaultEmbedMaxCostUSD,
	}
}

func (s *Service) Embed(ctx context.Context, o EmbedOptions, log *slog.Logger) (res EmbedResult, err error) {
	o.Model = cmp.Or(o.Model, DefaultEmbedModel)
	ctx, span := telemetry.Start(ctx, "service.Embed", attribute.String("collection", o.Collection), attribute.String("model", o.Model))
	defer func() { telemetry.End(span, err) }()
	log = s.logger(log).With("stage", StageEmbed, "model", o.Model)
	started := s.cfg.Now()

	db, err := s.catalog(ctx)
	if err != nil {
		return res, err
	}
	targets, err := db.PendingEmbeddings(ctx, store.PendingFilter{Model: o.Model, Collection: o.Collection, Limit: o.Limit})
	if err != nil {
		return res, err
	}
	res.Pending = len(targets)
	for _, t := range targets {
		res.Collages += len(t.Listing.Collages)
	}
	log.Info("listings without embeddings", "listings", res.Pending, "collages", res.Collages)
	if o.DryRun || len(targets) == 0 {
		return res, nil
	}

	key, err := s.cfg.Keys.OpenRouter()
	if err != nil {
		return res, err
	}
	client := s.cfg.Clients.Embedder(key)
	read := readShrunk(s.images())

	var (
		mu   sync.Mutex
		errs []error
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(cmpOr(o.Concurrency, DefaultEmbedConcurrency))
	overBudget := func() bool {
		mu.Lock()
		defer mu.Unlock()
		res.BudgetReached = res.BudgetReached || (o.MaxCostUSD > 0 && res.CostUSD >= o.MaxCostUSD)
		return res.BudgetReached
	}
	for i, t := range targets {
		if overBudget() || gctx.Err() != nil {
			break
		}
		g.Go(func() error {
			if overBudget() {
				return nil
			}
			e, err := embedOne(gctx, db, client, read, o, t, s.cfg.Now)
			mu.Lock()
			defer mu.Unlock()
			res.CostUSD += e.CostUSD
			res.Tokens += e.Tokens
			name := cmp.Or(t.Listing.Address.Formatted, string(t.Listing.Source)+"/"+t.Listing.SourceID)
			if err != nil {
				res.Failed++
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
				log.Warn("embedding failed", "step", fmt.Sprintf("%d/%d", i+1, len(targets)), "listing", name, "err", err)
				return nil
			}
			res.Embedded++
			log.Info("embedded", "step", fmt.Sprintf("%d/%d", i+1, len(targets)), "listing", name,
				"tokens", e.Tokens, "cost_usd", usd(e.CostUSD), "run_cost_usd", usd(res.CostUSD))
			return nil
		})
	}
	if werr := g.Wait(); werr != nil {
		errs = append(errs, werr)
	}
	if res.BudgetReached {
		log.Info("embedding budget reached; remaining listings will be embedded on the next run", "max_cost_usd", o.MaxCostUSD)
	}

	err = errors.Join(errs...)
	run := store.Run{
		Kind:       "embed",
		Collection: o.Collection,
		Day:        store.Day(started),
		StartedAt:  started,
		FinishedAt: s.cfg.Now(),
		Model:      o.Model,
		Calls:      res.Embedded + res.Failed,
		CostUSD:    res.CostUSD,
		Failed:     res.Failed,
	}
	run.Tokens.Prompt = res.Tokens
	if err != nil {
		run.Error = err.Error()
	}
	if rerr := db.RecordRun(context.WithoutCancel(ctx), run); rerr != nil {
		err = errors.Join(err, rerr)
	}
	return res, err
}

func embedOne(ctx context.Context, db *store.Store, client openrouter.Embedder, read func([]string) ([][]byte, error), o EmbedOptions, t store.EmbedTarget, now func() time.Time) (openrouter.Embedding, error) {
	collages, err := read(t.Listing.Collages)
	if err != nil {
		return openrouter.Embedding{}, err
	}
	var parts []openrouter.Part
	if t.Listing.Description != "" {
		parts = append(parts, openrouter.TextPart(t.Listing.Description))
	}
	for _, c := range collages {
		parts = append(parts, openrouter.ImagePart("image/jpeg", c))
	}
	e, err := client.Embed(ctx, openrouter.EmbedRequest{Model: o.Model, Parts: parts, Dimensions: int64(o.Dimensions)})
	if err != nil {
		return e, err
	}
	return e, db.SaveEmbedding(ctx, store.Embedding{
		VersionID: t.VersionID,
		Model:     o.Model,
		InputHash: t.InputHash,
		Vector:    e.Vector,
		Tokens:    e.Tokens,
		CostUSD:   e.CostUSD,
		CreatedAt: now(),
	})
}
