package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
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
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const DefaultMaxChargeUSD = 1.0

type FetchOptions struct {
	Collection       string
	Sources          []listing.Source
	Mode             profile.Mode
	Search           profile.Search
	Enrich           bool
	Photos           bool
	MaxChargeUSD     float64
	ApifyConcurrency int
}

type FetchResult struct {
	Day        string
	Listings   []listing.Listing
	Observed   int
	Distinct   int
	Duplicates int
}

func (s *Service) Fetch(ctx context.Context, o FetchOptions, log *slog.Logger) (FetchResult, error) {
	var res FetchResult
	log = s.logger(log)
	flog := log.With("stage", StageFetch)
	if o.Search.Location == "" {
		return res, fmt.Errorf("a location is required")
	}
	if o.Search.Limit <= 0 {
		return res, fmt.Errorf("the search limit must be positive")
	}
	if len(o.Sources) == 0 {
		return res, fmt.Errorf("at least one source is required")
	}
	q := o.Search.Query(o.Mode)
	res.Day = store.Day(s.cfg.Now())

	token, err := s.cfg.Keys.Apify()
	if err != nil {
		return res, err
	}
	runner := apify.Limit(s.cfg.Clients.Apify(token, o.MaxChargeUSD), cmpOr(o.ApifyConcurrency, apify.DefaultConcurrency))

	providers := make([]listing.Provider, len(o.Sources))
	for i, source := range o.Sources {
		p, err := s.cfg.Clients.Provider(source, runner)
		if err != nil {
			return res, err
		}
		if _, deferred := SplitAmenities(q.Amenities, p.SupportedAmenities(q.Offer)); len(deferred) > 0 {
			_, canEnrich := p.(listing.Enricher)
			switch {
			case canEnrich && !o.Enrich:
				return res, fmt.Errorf("%s cannot filter %s listings by %s; enable enrichment to filter on listing details instead", source, o.Mode, joinAmenities(deferred))
			case canEnrich:
				flog.Info("amenities checked after enrichment", "source", source, "amenities", joinAmenities(deferred))
			default:
				flog.Info("amenities checked against search results", "source", source, "amenities", joinAmenities(deferred))
			}
		}
		providers[i] = p
	}

	db := s.db

	results := make([][]listing.Listing, len(providers))
	g, gctx := errgroup.WithContext(ctx)
	for i, p := range providers {
		g.Go(func() error {
			ls, err := s.fetchOne(gctx, p, q, o, flog.With("source", p.Source()))
			if err != nil {
				return fmt.Errorf("%s: %w", p.Source(), err)
			}
			results[i] = ls
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return res, err
	}
	listings := slices.Concat(results...)

	stored, err := db.Load(ctx)
	if err != nil {
		return res, err
	}
	listings = CarryOver(stored, listings)

	if err := db.Observe(ctx, res.Day, o.Collection, listings); err != nil {
		return res, err
	}
	res.Observed = len(listings)
	flog.Info("stored observations", "listings", len(listings), "day", res.Day)

	distinct := FreshGroups(stored, listings)
	for _, g := range distinct {
		res.Duplicates += len(g.Others)
	}
	res.Distinct = len(distinct)
	if res.Duplicates > 0 {
		log.Info("deduplicated", "stage", StageDedupe, "distinct", res.Distinct, "duplicates", res.Duplicates)
	}
	listings = dedupe.Primaries(distinct)

	if o.Photos && len(listings) > 0 {
		p := media.NewProcessor(s.photoFetcher(), s.images(), media.NewGrid())
		p.Logger = log.With("stage", StagePhotos)
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return res, err
		}
		log.Info("photos downloaded", "stage", StagePhotos)
	}
	res.Listings = listings
	return res, nil
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

func (s *Service) fetchOne(ctx context.Context, p listing.Provider, q listing.Query, o FetchOptions, log *slog.Logger) ([]listing.Listing, error) {
	sourceQuery := q
	sourceQuery.Amenities, _ = SplitAmenities(q.Amenities, p.SupportedAmenities(q.Offer))

	listings, err := p.Search(ctx, sourceQuery)
	if err != nil {
		return nil, err
	}
	log.Info("searched", "mode", o.Mode, "listings", len(listings))

	stage := "search"
	if enricher, ok := p.(listing.Enricher); ok && o.Enrich && len(listings) > 0 {
		if listings, err = enricher.Enrich(ctx, listings); err != nil {
			return nil, err
		}
		stage = "enrich"
	}
	total := len(listings)
	matching := slices.DeleteFunc(listings, func(l listing.Listing) bool { return !q.Matches(l) })
	log.Info("filtered", "after", stage, "matching", len(matching), "total", total)
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

func ValidateSources(sources []listing.Source) error {
	for _, src := range sources {
		if _, err := NewProvider(src, nil); err != nil {
			return err
		}
	}
	return nil
}

func SupportedSources() []listing.Source {
	var out []listing.Source
	for _, src := range listing.Sources {
		if _, err := NewProvider(src, nil); err == nil {
			out = append(out, src)
		}
	}
	return out
}

const LookupSites = "Zillow, Redfin, Craigslist or Facebook Marketplace"

func LookupFor(rawURL string, runner apify.Runner) (listing.Lookup, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	var check func(string) (string, error)
	var lookup listing.Lookup
	switch host := strings.ToLower(u.Hostname()); {
	case host == "zillow.com" || strings.HasSuffix(host, ".zillow.com"):
		return zillow.New(runner, zillow.NewAutocomplete()), nil
	case redfin.IsHost(host):
		check, lookup = redfin.ListingURL, redfin.New(runner, zillow.NewAutocomplete())
	case craigslist.IsHost(host):
		check, lookup = craigslist.ListingURL, craigslist.New(runner, zillow.NewAutocomplete())
	case facebook.IsHost(host):
		check, lookup = facebook.ListingURL, facebook.New(runner, zillow.NewAutocomplete())
	default:
		return nil, fmt.Errorf("no lookup available for %q: use a listing from %s", u.Hostname(), LookupSites)
	}
	if _, err := check(rawURL); err != nil {
		return nil, err
	}
	return lookup, nil
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
