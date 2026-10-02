package streeteasy

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

const (
	ActorID         = "memo23/streeteasy-ppr"
	origin          = "https://streeteasy.com"
	defaultLimit    = 50
	maxZips         = 30
	overfetchFactor = 1.5
)

var (
	zipPattern    = regexp.MustCompile(`^\d{5}$`)
	filterEscaper = strings.NewReplacer("|", "%7C", ">", "%3E", "<", "%3C")
)

var regionRadiusMiles = map[string]float64{
	"neighborhood": 1,
	"zipcode":      1,
}

var amenityFilters = map[listing.Amenity]string{
	listing.AmenityInUnitLaundry: "washer_dryer",
	listing.AmenityDishwasher:    "dishwasher",
	listing.AmenityParking:       "parking",
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
	return listing.SourceStreetEasy
}

func (p *Provider) SupportedAmenities(listing.OfferType) []listing.Amenity {
	out := make([]listing.Amenity, 0, len(amenityFilters))
	for a := range amenityFilters {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}

type actorInput struct {
	StartURLs           []actorURL `json:"startUrls"`
	MaxItems            int        `json:"maxItems"`
	FlattenDatasetItems bool       `json:"flattenDatasetItems"`
	MonitoringMode      bool       `json:"monitoringMode"`
}

type actorURL struct {
	URL string `json:"url"`
}

type searchArea struct {
	center listing.Coordinates
	radius float64
	zips   []string
}

func (p *Provider) Search(ctx context.Context, q listing.Query) ([]listing.Listing, error) {
	if q.Offer != listing.OfferRent && q.Offer != listing.OfferSale {
		return nil, fmt.Errorf("%w: offer type %q", listing.ErrUnsupportedQuery, q.Offer)
	}
	if q.Area.Location == "" {
		return nil, fmt.Errorf("%w: location is required", listing.ErrUnsupportedQuery)
	}
	for _, a := range q.Amenities {
		if _, ok := amenityFilters[a]; !ok {
			return nil, fmt.Errorf("%w: streeteasy cannot filter by %s", listing.ErrUnsupportedQuery, a)
		}
	}

	area, err := p.resolve(ctx, q.Area)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	perZip := int(math.Ceil(float64(limit) * overfetchFactor / float64(len(area.zips))))

	results := make([][]json.RawMessage, len(area.zips))
	g, gctx := errgroup.WithContext(ctx)
	for i, zip := range area.zips {
		g.Go(func() error {
			input := actorInput{StartURLs: []actorURL{{URL: buildSearchURL(q, zip)}}, MaxItems: perZip}
			items, err := p.Runner.Run(gctx, ActorID, apify.RunInput{Input: input, MaxItems: perZip})
			results[i] = items
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	now := p.Now().UTC()
	seen := map[string]bool{}
	var out []listing.Listing
	for _, items := range results {
		for _, raw := range items {
			l, ok, err := toListing(raw, q.Offer, now)
			if err != nil {
				return nil, err
			}
			if !ok || seen[l.SourceID] || !q.Matches(l) || !withinAge(l, q.MaxAge, now) {
				continue
			}
			if area.radius > 0 && (l.Coordinates == nil || geo.DistanceMiles(area.center, *l.Coordinates) > area.radius) {
				continue
			}
			seen[l.SourceID] = true
			out = append(out, l)
		}
	}
	slices.SortStableFunc(out, newestFirst)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (p *Provider) resolve(ctx context.Context, a listing.Area) (searchArea, error) {
	location := strings.TrimSpace(a.Location)
	if zipPattern.MatchString(location) && a.RadiusMiles == 0 {
		return searchArea{zips: []string{location}}, nil
	}

	var center listing.Coordinates
	radius := a.RadiusMiles
	if z, ok := geo.LookupZip(location); ok {
		center = z.Center
	} else {
		region, err := p.Regions.Resolve(ctx, location)
		if err != nil {
			return searchArea{}, err
		}
		center = region.Center
		if radius == 0 {
			span, ok := regionRadiusMiles[region.Type]
			if !ok {
				return searchArea{}, fmt.Errorf("%w: %q is too large an area for streeteasy; set a radius", listing.ErrUnsupportedQuery, region.Name)
			}
			radius = span
		}
	}

	zips := geo.ZipsWithin(center, radius)
	if len(zips) == 0 {
		return searchArea{}, fmt.Errorf("%w: streeteasy only covers the New York City area, not %q", listing.ErrUnsupportedQuery, location)
	}
	if len(zips) > maxZips {
		return searchArea{}, fmt.Errorf("%w: a %.1f mile radius spans %d zip codes on streeteasy; the most is %d", listing.ErrUnsupportedQuery, radius, len(zips), maxZips)
	}
	area := searchArea{center: center, radius: radius}
	for _, z := range zips {
		area.zips = append(area.zips, z.Code)
	}
	return area, nil
}

func buildSearchURL(q listing.Query, zip string) string {
	filters := []string{"zip:" + zip}
	if q.MinPrice != nil || q.MaxPrice != nil {
		var lo, hi string
		if q.MinPrice != nil {
			lo = strconv.FormatInt(q.MinPrice.Cents/100, 10)
		}
		if q.MaxPrice != nil {
			hi = strconv.FormatInt((q.MaxPrice.Cents+99)/100, 10)
		}
		filters = append(filters, "price:"+lo+"-"+hi)
	}
	switch {
	case q.MinBeds != nil && q.MaxBeds != nil:
		filters = append(filters, fmt.Sprintf("beds:%d-%d", *q.MinBeds, *q.MaxBeds))
	case q.MinBeds != nil:
		filters = append(filters, fmt.Sprintf("beds>=%d", *q.MinBeds))
	case q.MaxBeds != nil:
		filters = append(filters, fmt.Sprintf("beds:0-%d", *q.MaxBeds))
	}
	var amenities []string
	for _, a := range q.Amenities {
		amenities = append(amenities, amenityFilters[a])
	}
	if len(amenities) > 0 {
		slices.Sort(amenities)
		filters = append(filters, "amenities:"+strings.Join(amenities, ","))
	}

	kind := "for-rent"
	if q.Offer == listing.OfferSale {
		kind = "for-sale"
	}
	path := filterEscaper.Replace(strings.Join(filters, "|"))
	return origin + "/" + kind + "/nyc/" + path + "?sort_by=listed_desc"
}

func withinAge(l listing.Listing, maxAge time.Duration, now time.Time) bool {
	if maxAge <= 0 || l.ListedAt == nil {
		return true
	}
	return now.Sub(*l.ListedAt) <= maxAge
}

func newestFirst(a, b listing.Listing) int {
	switch {
	case a.ListedAt == nil && b.ListedAt == nil:
		return 0
	case a.ListedAt == nil:
		return 1
	case b.ListedAt == nil:
		return -1
	}
	return cmp.Compare(b.ListedAt.Unix(), a.ListedAt.Unix())
}
