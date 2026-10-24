package craigslist

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

const (
	ActorID         = "memo23/craigslist-scraper"
	defaultMaxItems = 200
	maxRegionMiles  = 75
)

var zipPattern = regexp.MustCompile(`^\d{5}$`)

var regionRadiusMiles = map[string]float64{
	"neighborhood": 1.5,
	"zipcode":      2,
	"city":         4,
	"county":       15,
}

var amenityParams = map[listing.Amenity][]string{
	listing.AmenityInUnitLaundry:   {"laundry=1"},
	listing.AmenityParking:         {"parking=1", "parking=2", "parking=3", "parking=4", "parking=6"},
	listing.AmenityAirConditioning: {"airconditioning=1"},
}

type subdomain struct {
	name   string
	center listing.Coordinates
}

var subdomains = []subdomain{
	{"boston", listing.Coordinates{Lat: 42.3601, Lng: -71.0589}},
	{"worcester", listing.Coordinates{Lat: 42.2626, Lng: -71.8023}},
	{"westernmass", listing.Coordinates{Lat: 42.1015, Lng: -72.5898}},
	{"providence", listing.Coordinates{Lat: 41.8240, Lng: -71.4128}},
	{"nh", listing.Coordinates{Lat: 42.9956, Lng: -71.4548}},
	{"newyork", listing.Coordinates{Lat: 40.7128, Lng: -74.0060}},
	{"philadelphia", listing.Coordinates{Lat: 39.9526, Lng: -75.1652}},
	{"washingtondc", listing.Coordinates{Lat: 38.9072, Lng: -77.0369}},
	{"atlanta", listing.Coordinates{Lat: 33.7490, Lng: -84.3880}},
	{"miami", listing.Coordinates{Lat: 25.7617, Lng: -80.1918}},
	{"chicago", listing.Coordinates{Lat: 41.8781, Lng: -87.6298}},
	{"minneapolis", listing.Coordinates{Lat: 44.9778, Lng: -93.2650}},
	{"denver", listing.Coordinates{Lat: 39.7392, Lng: -104.9903}},
	{"austin", listing.Coordinates{Lat: 30.2672, Lng: -97.7431}},
	{"dallas", listing.Coordinates{Lat: 32.7767, Lng: -96.7970}},
	{"houston", listing.Coordinates{Lat: 29.7604, Lng: -95.3698}},
	{"phoenix", listing.Coordinates{Lat: 33.4484, Lng: -112.0740}},
	{"losangeles", listing.Coordinates{Lat: 34.0522, Lng: -118.2437}},
	{"sandiego", listing.Coordinates{Lat: 32.7157, Lng: -117.1611}},
	{"sfbay", listing.Coordinates{Lat: 37.7749, Lng: -122.4194}},
	{"portland", listing.Coordinates{Lat: 45.5152, Lng: -122.6784}},
	{"seattle", listing.Coordinates{Lat: 47.6062, Lng: -122.3321}},
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
	return listing.SourceCraigslist
}

func (p *Provider) SupportedAmenities(offer listing.OfferType) []listing.Amenity {
	if offer != listing.OfferRent {
		return nil
	}
	out := make([]listing.Amenity, 0, len(amenityParams))
	for a := range amenityParams {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}

type actorInput struct {
	StartURLs      []actorURL `json:"startUrls"`
	IncludeDetails bool       `json:"includeDetails"`
	MaxItems       int        `json:"maxItems"`
}

type actorURL struct {
	URL string `json:"url"`
}

type searchArea struct {
	subdomain string
	postal    string
	center    listing.Coordinates
	radius    float64
	exact     bool
}

func (p *Provider) Search(ctx context.Context, q listing.Query) ([]listing.Listing, error) {
	if q.Offer != listing.OfferRent {
		return nil, fmt.Errorf("%w: craigslist supports rentals only, not %q", listing.ErrUnsupportedQuery, q.Offer)
	}
	if q.Area.Location == "" {
		return nil, fmt.Errorf("%w: location is required", listing.ErrUnsupportedQuery)
	}
	supported := p.SupportedAmenities(q.Offer)
	for _, a := range q.Amenities {
		if !slices.Contains(supported, a) {
			return nil, fmt.Errorf("%w: craigslist cannot filter by %s", listing.ErrUnsupportedQuery, a)
		}
	}

	area, err := p.resolve(ctx, q.Area)
	if err != nil {
		return nil, err
	}
	maxItems := q.Limit
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}
	items, err := p.Runner.Run(ctx, ActorID, actorInput{
		StartURLs:      []actorURL{{URL: buildSearchURL(q, area)}},
		IncludeDetails: true,
		MaxItems:       maxItems,
	})
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
		if !ok || !q.Matches(l) || !withinAge(l, q.MaxAge, now) {
			continue
		}
		if area.exact && (l.Coordinates == nil || geo.DistanceMiles(area.center, *l.Coordinates) > area.radius) {
			continue
		}
		out = append(out, l)
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func (p *Provider) resolve(ctx context.Context, a listing.Area) (searchArea, error) {
	location := strings.TrimSpace(a.Location)
	region, err := p.Regions.Resolve(ctx, location)
	if err != nil {
		return searchArea{}, err
	}
	sub, ok := nearestSubdomain(region.Center)
	if !ok {
		return searchArea{}, fmt.Errorf("%w: no craigslist region near %q", listing.ErrUnsupportedQuery, region.Name)
	}
	area := searchArea{subdomain: sub, center: region.Center, radius: a.RadiusMiles, exact: a.RadiusMiles > 0}
	if area.radius == 0 {
		span, ok := regionRadiusMiles[region.Type]
		if !ok {
			return searchArea{}, fmt.Errorf("%w: %q is too large an area for craigslist; set a radius", listing.ErrUnsupportedQuery, region.Name)
		}
		area.radius = span
	}
	if zipPattern.MatchString(location) {
		area.postal = location
	}
	return area, nil
}

func nearestSubdomain(c listing.Coordinates) (string, bool) {
	best, bestMiles := "", math.Inf(1)
	for _, s := range subdomains {
		if d := geo.DistanceMiles(c, s.center); d < bestMiles {
			best, bestMiles = s.name, d
		}
	}
	return best, bestMiles <= maxRegionMiles
}

func buildSearchURL(q listing.Query, a searchArea) string {
	params := []string{"sort=date", "hasPic=1"}
	if q.MinPrice != nil {
		params = append(params, "min_price="+strconv.FormatInt(q.MinPrice.Cents/100, 10))
	}
	if q.MaxPrice != nil {
		params = append(params, "max_price="+strconv.FormatInt((q.MaxPrice.Cents+99)/100, 10))
	}
	if q.MinBeds != nil {
		params = append(params, "min_bedrooms="+strconv.Itoa(*q.MinBeds))
	}
	if q.MaxBeds != nil {
		params = append(params, "max_bedrooms="+strconv.Itoa(*q.MaxBeds))
	}
	if q.MaxAge > 0 && q.MaxAge <= 24*time.Hour {
		params = append(params, "postedToday=1")
	}
	for _, am := range q.Amenities {
		params = append(params, amenityParams[am]...)
	}

	radius := strconv.FormatFloat(math.Max(a.radius, 0.5), 'f', -1, 64)
	if a.postal != "" {
		params = append(params, "postal="+url.QueryEscape(a.postal), "search_distance="+radius)
	} else {
		params = append(params,
			"lat="+strconv.FormatFloat(a.center.Lat, 'f', 4, 64),
			"lon="+strconv.FormatFloat(a.center.Lng, 'f', 4, 64),
			"search_distance="+radius,
		)
	}
	return fmt.Sprintf("https://%s.craigslist.org/search/apa?%s", a.subdomain, strings.Join(params, "&"))
}

func withinAge(l listing.Listing, maxAge time.Duration, now time.Time) bool {
	if maxAge <= 0 || l.ListedAt == nil {
		return true
	}
	return now.Sub(*l.ListedAt) <= maxAge
}
