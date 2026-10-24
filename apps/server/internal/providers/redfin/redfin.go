package redfin

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

const ActorID = "tri_angle/redfin-search"

const day = 24 * time.Hour

var zipPattern = regexp.MustCompile(`^\d{5}$`)

var regionRadiusMiles = map[string]float64{
	"neighborhood": 1.5,
	"zipcode":      2,
	"city":         4,
	"county":       15,
}

var daysOnMarketBuckets = []struct {
	max   time.Duration
	value string
}{
	{7 * day, "1wk"},
	{14 * day, "2wk"},
	{30 * day, "1mo"},
	{90 * day, "3mo"},
	{180 * day, "6mo"},
	{365 * day, "1yr"},
}

type Provider struct {
	Runner  apify.Runner
	Regions zillow.RegionResolver
	Now     func() time.Time
}

func New(runner apify.Runner, regions zillow.RegionResolver) *Provider {
	return &Provider{Runner: runner, Regions: regions, Now: time.Now}
}

var _ listing.Provider = (*Provider)(nil)

func (p *Provider) Source() listing.Source {
	return listing.SourceRedfin
}

func (p *Provider) SupportedAmenities(listing.OfferType) []listing.Amenity {
	return nil
}

type actorInput struct {
	SearchURLs []actorURL `json:"searchUrls"`
	MaxResults int        `json:"maxResults,omitempty"`
	ZoomIn     bool       `json:"zoomIn"`
}

type actorURL struct {
	URL string `json:"url"`
}

func (p *Provider) Search(ctx context.Context, q listing.Query) ([]listing.Listing, error) {
	if q.Offer != listing.OfferRent && q.Offer != listing.OfferSale {
		return nil, fmt.Errorf("%w: offer type %q", listing.ErrUnsupportedQuery, q.Offer)
	}
	if q.Area.Location == "" {
		return nil, fmt.Errorf("%w: location is required", listing.ErrUnsupportedQuery)
	}
	if len(q.Amenities) > 0 {
		return nil, fmt.Errorf("%w: redfin cannot filter by amenities", listing.ErrUnsupportedQuery)
	}

	area, err := p.resolve(ctx, q.Area)
	if err != nil {
		return nil, err
	}
	items, err := p.Runner.Run(ctx, ActorID, actorInput{
		SearchURLs: []actorURL{{URL: buildSearchURL(q, area)}},
		MaxResults: q.Limit,
		ZoomIn:     true,
	})
	if err != nil {
		return nil, err
	}

	now := p.Now().UTC()
	var out []listing.Listing
	for _, raw := range items {
		l, ok, err := toListing(raw, q, now)
		if err != nil {
			return nil, err
		}
		if !ok || !q.Matches(l) {
			continue
		}
		if area.radius > 0 && (l.Coordinates == nil || geo.DistanceMiles(area.center, *l.Coordinates) > area.radius) {
			continue
		}
		out = append(out, l)
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

type searchArea struct {
	zip      string
	outline  bool
	center   listing.Coordinates
	bounds   geo.Bounds
	radius   float64
	viewport bool
}

func (p *Provider) resolve(ctx context.Context, a listing.Area) (searchArea, error) {
	location := strings.TrimSpace(a.Location)
	if zipPattern.MatchString(location) && a.RadiusMiles == 0 {
		return searchArea{zip: location, outline: true}, nil
	}

	region, err := p.Regions.Resolve(ctx, location)
	if err != nil {
		return searchArea{}, err
	}
	radius := a.RadiusMiles
	if radius == 0 {
		span, ok := regionRadiusMiles[region.Type]
		if !ok {
			return searchArea{}, fmt.Errorf("%w: %q is too large an area for redfin; set a radius", listing.ErrUnsupportedQuery, region.Name)
		}
		radius = span
	}
	area := searchArea{
		zip:      viewportAnchorZip,
		center:   region.Center,
		bounds:   geo.BoundsAround(region.Center, radius),
		viewport: true,
	}
	if a.RadiusMiles > 0 {
		area.radius = a.RadiusMiles
	}
	if zipPattern.MatchString(location) {
		area.zip = location
	}
	return area, nil
}

const viewportAnchorZip = "10001"

func buildSearchURL(q listing.Query, a searchArea) string {
	var filters []string
	if q.MinPrice != nil {
		filters = append(filters, "min-price="+strconv.FormatInt(q.MinPrice.Cents/100, 10))
	}
	if q.MaxPrice != nil {
		filters = append(filters, "max-price="+strconv.FormatInt((q.MaxPrice.Cents+99)/100, 10))
	}
	if q.MinBeds != nil {
		filters = append(filters, "min-beds="+strconv.Itoa(*q.MinBeds))
	}
	if q.MaxBeds != nil {
		filters = append(filters, "max-beds="+strconv.Itoa(*q.MaxBeds))
	}
	if q.Offer == listing.OfferSale {
		if v, ok := daysOnMarket(q.MaxAge); ok {
			filters = append(filters, "max-days-on-market="+v)
		}
	}
	if a.viewport {
		b := a.bounds
		filters = append(filters, fmt.Sprintf("viewport=%.5f:%.5f:%.5f:%.5f", b.North, b.South, b.East, b.West), "no-outline")
	}

	u := "https://www.redfin.com/zipcode/" + a.zip
	if q.Offer == listing.OfferRent {
		u += "/apartments-for-rent"
	}
	if len(filters) > 0 {
		u += "/filter/" + strings.Join(filters, ",")
	}
	return u
}

func daysOnMarket(maxAge time.Duration) (string, bool) {
	if maxAge <= 0 {
		return "", false
	}
	for _, b := range daysOnMarketBuckets {
		if maxAge <= b.max {
			return b.value, true
		}
	}
	return "", false
}
