package facebook

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

const (
	ActorID     = "curious_coder/facebook-marketplace"
	anchorSlug  = "boston"
	kmPerMile   = 1.609344
	day         = 24 * time.Hour
	extraFactor = 2
)

var regionRadiusMiles = map[string]float64{
	"neighborhood": 2,
	"zipcode":      3,
	"city":         5,
	"county":       15,
}

var daysSinceListedBuckets = []struct {
	max   time.Duration
	value string
}{
	{1 * day, "1"},
	{7 * day, "7"},
	{30 * day, "30"},
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
	return listing.SourceFacebook
}

func (p *Provider) SupportedAmenities(listing.OfferType) []listing.Amenity {
	return nil
}

type actorInput struct {
	URLs                []string `json:"urls"`
	GetListingDetails   bool     `json:"getListingDetails"`
	GetAllListingPhotos bool     `json:"getAllListingPhotos"`
	Proxy               proxy    `json:"proxy"`
}

type proxy struct {
	UseApifyProxy     bool   `json:"useApifyProxy"`
	ApifyProxyCountry string `json:"apifyProxyCountry"`
}

func (p *Provider) Search(ctx context.Context, q listing.Query) ([]listing.Listing, error) {
	if q.Offer != listing.OfferRent {
		return nil, fmt.Errorf("%w: facebook marketplace only supports rentals", listing.ErrUnsupportedQuery)
	}
	if q.Area.Location == "" {
		return nil, fmt.Errorf("%w: location is required", listing.ErrUnsupportedQuery)
	}
	if len(q.Amenities) > 0 {
		return nil, fmt.Errorf("%w: facebook marketplace cannot filter by amenities", listing.ErrUnsupportedQuery)
	}

	region, err := p.Regions.Resolve(ctx, q.Area.Location)
	if err != nil {
		return nil, err
	}
	radius := q.Area.RadiusMiles
	if radius == 0 {
		span, ok := regionRadiusMiles[region.Type]
		if !ok {
			return nil, fmt.Errorf("%w: %q is too large an area for facebook marketplace; set a radius", listing.ErrUnsupportedQuery, region.Name)
		}
		radius = span
	}

	var run any = actorInput{
		URLs:                []string{searchURL(q, region.Center, radius)},
		GetListingDetails:   true,
		GetAllListingPhotos: true,
		Proxy:               proxy{UseApifyProxy: true, ApifyProxyCountry: "US"},
	}
	if q.Limit > 0 {
		run = apify.RunInput{Input: run, MaxItems: q.Limit * extraFactor}
	}
	items, err := p.Runner.Run(ctx, ActorID, run)
	if err != nil {
		return nil, err
	}

	now := p.Now().UTC()
	var out []listing.Listing
	for _, raw := range items {
		l, ok, err := toListing(raw, now)
		if err != nil {
			return nil, err
		}
		if !ok || !q.Matches(l) {
			continue
		}
		if l.Coordinates != nil && geo.DistanceMiles(region.Center, *l.Coordinates) > radius {
			continue
		}
		if q.MaxAge > 0 && l.ListedAt != nil && now.Sub(*l.ListedAt) > q.MaxAge {
			continue
		}
		out = append(out, l)
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func searchURL(q listing.Query, center listing.Coordinates, radiusMiles float64) string {
	v := url.Values{}
	v.Set("sortBy", "creation_time_descend")
	v.Set("exact", "false")
	v.Set("latitude", strconv.FormatFloat(center.Lat, 'f', 5, 64))
	v.Set("longitude", strconv.FormatFloat(center.Lng, 'f', 5, 64))
	v.Set("radius", strconv.Itoa(int(math.Ceil(radiusMiles*kmPerMile))))
	if q.MinPrice != nil {
		v.Set("minPrice", strconv.FormatInt(q.MinPrice.Cents/100, 10))
	}
	if q.MaxPrice != nil {
		v.Set("maxPrice", strconv.FormatInt((q.MaxPrice.Cents+99)/100, 10))
	}
	if q.MinBeds != nil {
		v.Set("minBedrooms", strconv.Itoa(*q.MinBeds))
	}
	if q.MaxBeds != nil {
		v.Set("maxBedrooms", strconv.Itoa(*q.MaxBeds))
	}
	if d, ok := daysSinceListed(q.MaxAge); ok {
		v.Set("daysSinceListed", d)
	}
	return "https://www.facebook.com/marketplace/" + anchorSlug + "/propertyrentals?" + v.Encode()
}

func daysSinceListed(maxAge time.Duration) (string, bool) {
	if maxAge <= 0 {
		return "", false
	}
	for _, b := range daysSinceListedBuckets {
		if maxAge <= b.max {
			return b.value, true
		}
	}
	return "", false
}
