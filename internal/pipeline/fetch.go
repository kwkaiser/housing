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
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/dedupe"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/craigslist"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/facebook"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/redfin"
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
			_, canEnrich := p.(listing.Enricher)
			switch {
			case canEnrich && !o.Enrich:
				return nil, fmt.Errorf("%s cannot filter %s listings by %s; enable enrichment to filter on listing details instead", source, o.Mode, joinAmenities(deferred))
			case canEnrich:
				e.printf("%s: amenities checked after enrichment: %s\n", source, joinAmenities(deferred))
			default:
				e.printf("%s: amenities checked against search results: %s\n", source, joinAmenities(deferred))
			}
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

	stored, err := e.persister().Load(ctx, e.DataDir)
	if err != nil {
		return nil, err
	}
	listings = CarryOver(stored, listings)

	if err := e.persister().Persist(ctx, e.DataDir, listings); err != nil {
		return nil, err
	}
	e.printf("stored: %d listings in %s\n", len(listings), e.DataDir)

	distinct := FreshGroups(stored, listings)
	dupes := 0
	for _, g := range distinct {
		dupes += len(g.Others)
	}
	if dupes > 0 {
		e.printf("dedupe: %d distinct listings, %d duplicates across sources\n", len(distinct), dupes)
	}
	listings = dedupe.Primaries(distinct)

	if o.Photos && len(listings) > 0 {
		p := media.NewProcessor(media.NewHTTPFetcher(), e.images(), media.NewGrid())
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return nil, err
		}
		e.printf("photos: downloaded\n")
	}
	return listings, nil
}

func FreshGroups(stored, fresh []listing.Listing) []dedupe.Group {
	isFresh := make(map[string]bool, len(fresh))
	for _, l := range fresh {
		isFresh[dedupe.Key(l)] = true
	}
	all := slices.DeleteFunc(slices.Clone(stored), func(l listing.Listing) bool { return isFresh[dedupe.Key(l)] })
	all = append(all, fresh...)

	var out []dedupe.Group
	for _, g := range dedupe.Groups(all) {
		if slices.ContainsFunc(g.Members(), func(l listing.Listing) bool { return isFresh[dedupe.Key(l)] }) {
			out = append(out, g)
		}
	}
	return out
}

func CarryOver(stored, fresh []listing.Listing) []listing.Listing {
	byKey := make(map[string]listing.Listing, len(stored))
	for _, l := range stored {
		byKey[string(l.Source)+"/"+l.SourceID] = l
	}
	out := make([]listing.Listing, len(fresh))
	for i, l := range fresh {
		if prev, ok := byKey[string(l.Source)+"/"+l.SourceID]; ok {
			if l.Assessments == nil {
				l.Assessments = prev.Assessments
			}
			if l.Collages == nil {
				l.Collages = prev.Collages
			}
		}
		out[i] = l
	}
	return out
}

func (e *Env) fetchOne(ctx context.Context, p listing.Provider, q listing.Query, o FetchOptions) ([]listing.Listing, error) {
	sourceQuery := q
	sourceQuery.Amenities, _ = SplitAmenities(q.Amenities, p.SupportedAmenities(q.Offer))

	listings, err := p.Search(ctx, sourceQuery)
	if err != nil {
		return nil, err
	}
	e.printf("%s search (%s): %d listings\n", p.Source(), o.Mode, len(listings))

	stage := "search"
	if enricher, ok := p.(listing.Enricher); ok && o.Enrich && len(listings) > 0 {
		if listings, err = enricher.Enrich(ctx, listings); err != nil {
			return nil, err
		}
		stage = "enrich"
	}
	total := len(listings)
	matching := slices.DeleteFunc(listings, func(l listing.Listing) bool { return !q.Matches(l) })
	e.printf("%s %s: %d of %d listings match\n", p.Source(), stage, len(matching), total)
	return matching, nil
}

func NewProvider(source listing.Source, runner apify.Runner) (listing.Provider, error) {
	switch source {
	case listing.SourceZillow:
		return zillow.New(runner, zillow.NewAutocomplete()), nil
	case listing.SourceRedfin:
		return redfin.New(runner, zillow.NewAutocomplete()), nil
	case listing.SourceCraigslist:
		return craigslist.New(runner, zillow.NewAutocomplete()), nil
	case listing.SourceFacebook:
		return facebook.New(runner, zillow.NewAutocomplete()), nil
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
