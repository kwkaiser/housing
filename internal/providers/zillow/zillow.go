package zillow

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const ActorID = "maxcopell/zillow-scraper"

var rentalAmenityFilters = map[listing.Amenity]string{
	listing.AmenityInUnitLaundry:   "lau",
	listing.AmenityParking:         "parka",
	listing.AmenityAirConditioning: "ac",
}

type Provider struct {
	Runner  apify.Runner
	Regions RegionResolver
}

func New(runner apify.Runner, regions RegionResolver) *Provider {
	return &Provider{Runner: runner, Regions: regions}
}

func (p *Provider) Source() listing.Source {
	return listing.SourceZillow
}

func (p *Provider) SupportedAmenities(offer listing.OfferType) []listing.Amenity {
	if offer != listing.OfferRent {
		return nil
	}
	return slices.Sorted(maps.Keys(rentalAmenityFilters))
}

func (p *Provider) Search(ctx context.Context, q listing.Query) ([]listing.Listing, error) {
	if err := p.validate(q); err != nil {
		return nil, err
	}

	region, err := p.Regions.Resolve(ctx, q.Area.Location)
	if err != nil {
		return nil, err
	}

	searchURL, err := buildSearchURL(q, region)
	if err != nil {
		return nil, err
	}

	items, err := p.Runner.Run(ctx, ActorID, actorInput{
		SearchURLs:       []actorURL{{URL: searchURL}},
		ExtractionMethod: "PAGINATION_WITH_ZOOM_IN",
		ResultsLimit:     q.Limit,
	})
	if err != nil {
		return nil, err
	}

	var out []listing.Listing
	for _, raw := range items {
		ls, err := toListings(raw, q)
		if err != nil {
			return nil, err
		}
		for _, l := range ls {
			if matches(l, q, region) {
				out = append(out, l)
			}
		}
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func (p *Provider) validate(q listing.Query) error {
	if q.Offer != listing.OfferRent && q.Offer != listing.OfferSale {
		return fmt.Errorf("%w: offer type %q", listing.ErrUnsupportedQuery, q.Offer)
	}
	if q.Area.Location == "" {
		return fmt.Errorf("%w: location is required", listing.ErrUnsupportedQuery)
	}
	for _, m := range []*listing.Money{q.MinPrice, q.MaxPrice} {
		if m != nil && m.Currency != "" && m.Currency != "USD" {
			return fmt.Errorf("%w: currency %q", listing.ErrUnsupportedQuery, m.Currency)
		}
	}
	supported := p.SupportedAmenities(q.Offer)
	for _, a := range q.Amenities {
		if !slices.Contains(supported, a) {
			return fmt.Errorf("%w: zillow cannot filter %s listings by %s", listing.ErrUnsupportedQuery, q.Offer, a)
		}
	}
	return nil
}

func matches(l listing.Listing, q listing.Query, region Region) bool {
	if !q.Matches(l) {
		return false
	}
	if q.Area.RadiusMiles > 0 {
		return l.Coordinates != nil && geo.DistanceMiles(region.Center, *l.Coordinates) <= q.Area.RadiusMiles
	}
	return true
}

type actorInput struct {
	SearchURLs       []actorURL `json:"searchUrls"`
	ExtractionMethod string     `json:"extractionMethod"`
	ResultsLimit     int        `json:"resultsLimit,omitempty"`
}

type actorURL struct {
	URL string `json:"url"`
}

var (
	_ listing.Provider = (*Provider)(nil)
	_ listing.Enricher = (*Provider)(nil)
)
