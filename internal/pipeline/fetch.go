package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

const DefaultFetchLimit = 10

var ErrNoSavedSearch = errors.New("no saved search")

type FetchOptions struct {
	Sources          []listing.Source
	Mode             profile.Mode
	Search           profile.Search
	Enrich           bool
	Photos           bool
	MaxChargeUSD     float64
	ApifyConcurrency int
}

func (e *Env) SavedSearch(profileID string, mode profile.Mode) (profile.Search, error) {
	base := profile.Search{Limit: DefaultFetchLimit}
	if profileID == "" {
		return base, nil
	}
	p, err := e.profiles().Load(profileID)
	if err != nil {
		return profile.Search{}, err
	}
	saved, ok := p.Searches[mode]
	if !ok {
		return profile.Search{Limit: DefaultFetchLimit}, fmt.Errorf("%w: profile %q has no %s search; set one with `housing profile search %s --mode %s ...` or pass --location", ErrNoSavedSearch, p.ID, mode, p.ID, mode)
	}
	if saved.Limit == 0 {
		saved.Limit = DefaultFetchLimit
	}
	return saved, nil
}

func (e *Env) Fetch(ctx context.Context, o FetchOptions) ([]listing.Listing, error) {
	if o.Search.Location == "" {
		return nil, fmt.Errorf("a location is required")
	}
	if o.Search.Limit <= 0 {
		return nil, fmt.Errorf("the search limit must be positive")
	}
	if len(o.Sources) == 0 {
		return nil, fmt.Errorf("at least one source is required")
	}
	q := o.Search.Query(o.Mode)

	token, err := e.Config.Apify()
	if err != nil {
		return nil, err
	}
	client := apify.NewClient(token)
	client.MaxTotalChargeUSD = o.MaxChargeUSD
	runner := apify.Limit(client, cmpOr(o.ApifyConcurrency, apify.DefaultConcurrency))

	providers := make([]listing.Provider, len(o.Sources))
	for i, source := range o.Sources {
		p, err := NewProvider(source, runner)
		if err != nil {
			return nil, err
		}
		if _, deferred := SplitAmenities(q.Amenities, p.SupportedAmenities(q.Offer)); len(deferred) > 0 {
			if _, ok := p.(listing.Enricher); !ok || !o.Enrich {
				return nil, fmt.Errorf("%s cannot filter %s listings by %s; enable enrichment to filter on listing details instead", source, o.Mode, joinAmenities(deferred))
			}
			e.printf("%s: amenities checked after enrichment: %s\n", source, joinAmenities(deferred))
		}
		providers[i] = p
	}

	results := make([][]listing.Listing, len(providers))
	g, gctx := errgroup.WithContext(ctx)
	for i, p := range providers {
		g.Go(func() error {
			ls, err := e.fetchOne(gctx, p, q, o)
			if err != nil {
				return fmt.Errorf("%s: %w", p.Source(), err)
			}
			results[i] = ls
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	listings := slices.Concat(results...)

	if o.Photos && len(listings) > 0 {
		p := media.NewProcessor(media.NewHTTPFetcher(), e.images(), media.NewGrid())
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return nil, err
		}
		e.printf("photos: downloaded\n")
	}

	if err := e.persister().Persist(ctx, e.DataDir, listings); err != nil {
		return nil, err
	}
	e.printf("stored: %d listings in %s\n", len(listings), e.DataDir)
	return listings, nil
}

func (e *Env) fetchOne(ctx context.Context, p listing.Provider, q listing.Query, o FetchOptions) ([]listing.Listing, error) {
	sourceQuery := q
	sourceQuery.Amenities, _ = SplitAmenities(q.Amenities, p.SupportedAmenities(q.Offer))

	listings, err := p.Search(ctx, sourceQuery)
	if err != nil {
		return nil, err
	}
	e.printf("%s search (%s): %d listings\n", p.Source(), o.Mode, len(listings))

	enricher, ok := p.(listing.Enricher)
	if !ok || !o.Enrich || len(listings) == 0 {
		return listings, nil
	}
	enriched, err := enricher.Enrich(ctx, listings)
	if err != nil {
		return nil, err
	}
	matching := slices.DeleteFunc(enriched, func(l listing.Listing) bool { return !q.Matches(l) })
	e.printf("%s enrich: %d of %d listings match\n", p.Source(), len(matching), len(enriched))
	return matching, nil
}

func NewProvider(source listing.Source, runner apify.Runner) (listing.Provider, error) {
	switch source {
	case listing.SourceZillow:
		return zillow.New(runner, zillow.NewAutocomplete()), nil
	}
	return nil, fmt.Errorf("unsupported source %q", source)
}

func SplitAmenities(want, supported []listing.Amenity) (filterable, deferred []listing.Amenity) {
	for _, a := range want {
		if slices.Contains(supported, a) {
			filterable = append(filterable, a)
		} else {
			deferred = append(deferred, a)
		}
	}
	return filterable, deferred
}

func joinAmenities(as []listing.Amenity) string {
	names := make([]string, len(as))
	for i, a := range as {
		names[i] = string(a)
	}
	return strings.Join(names, ", ")
}

func cmpOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
