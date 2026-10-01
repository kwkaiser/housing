package redfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const (
	origin             = "https://www.redfin.com"
	rentalLookupMiles  = 0.25
	rentalLookupLimit  = 50
	rentalLookupFilter = "viewport=%.5f:%.5f:%.5f:%.5f,no-outline"
)

var (
	_           listing.Lookup = (*Provider)(nil)
	listingPath                = regexp.MustCompile(`^/[A-Za-z]{2}/[^/]+/[^/]+(?:/[^/]+)?/(?:home|apartment)/(\d+)$`)
)

type addressSection struct {
	PriceInfo struct {
		Amount float64 `json:"amount"`
		Label  string  `json:"label"`
	} `json:"priceInfo"`
	SqFt          value[float64] `json:"sqFt"`
	StreetAddress struct {
		Number    string `json:"streetNumber"`
		Prefix    string `json:"directionalPrefix"`
		Name      string `json:"streetName"`
		Type      string `json:"streetType"`
		Suffix    string `json:"directionalSuffix"`
		UnitValue string `json:"unitValue"`
	} `json:"streetAddress"`
	LatLong      *coordinates `json:"latLong"`
	Beds         *float64     `json:"beds"`
	Baths        *float64     `json:"baths"`
	City         string       `json:"city"`
	State        string       `json:"state"`
	Zip          string       `json:"zip"`
	DaysOnMarket *int         `json:"cumulativeDaysOnMarket"`
	AVM          struct {
		PropertyID flexString `json:"propertyId"`
	} `json:"avmInfo"`
	URL string `json:"url"`
}

func IsHost(host string) bool {
	host = strings.ToLower(host)
	return host == "redfin.com" || strings.HasSuffix(host, ".redfin.com")
}

func ListingURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !IsHost(u.Hostname()) {
		return "", fmt.Errorf("not a redfin url: %q", raw)
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	if !listingPath.MatchString(path) {
		return "", fmt.Errorf("not a redfin listing page: %q; use a property link ending in /home/<id> or /apartment/<id>", raw)
	}
	return origin + path, nil
}

func (p *Provider) Lookup(ctx context.Context, rawURL string) (listing.Listing, error) {
	u, err := ListingURL(rawURL)
	if err != nil {
		return listing.Listing{}, err
	}
	id := listingPath.FindStringSubmatch(strings.TrimPrefix(u, origin))[1]

	items, err := p.Runner.Run(ctx, DetailActorID, detailInput{DetailURLs: []actorURL{{URL: u}}})
	if err != nil {
		return listing.Listing{}, err
	}
	d, raw, err := pickDetail(items, u, id)
	if err != nil {
		return listing.Listing{}, err
	}

	now := p.Now().UTC()
	if d.forSale() {
		return enrichSale(d.sale(u, id, raw, now), d), nil
	}
	return p.lookupRental(ctx, u, id, d, now)
}

func pickDetail(items []json.RawMessage, u, id string) (detailItem, json.RawMessage, error) {
	for _, raw := range items {
		var d detailItem
		if err := json.Unmarshal(raw, &d); err != nil {
			return detailItem{}, nil, fmt.Errorf("decode redfin detail: %w", err)
		}
		if d.Error != "" || d.Address.LatLong == nil {
			continue
		}
		if string(d.Address.AVM.PropertyID) == id || d.Input == u {
			return d, raw, nil
		}
	}
	return detailItem{}, nil, fmt.Errorf("redfin detail scraper returned nothing for %s", u)
}

func (d detailItem) forSale() bool {
	price := d.Address.PriceInfo
	return price.Amount > 0 && d.MainHouseInfo.ListingID > 0 && !strings.Contains(strings.ToLower(price.Label), "sold")
}

func (d detailItem) sale(u, id string, raw json.RawMessage, now time.Time) listing.Listing {
	a := d.Address
	s := a.StreetAddress
	street := strings.Join(strings.Fields(strings.Join([]string{s.Number, s.Prefix, s.Name, s.Type, s.Suffix}, " ")), " ")
	l := listing.Listing{
		Source:      listing.SourceRedfin,
		SourceID:    id,
		URL:         u,
		Offer:       listing.OfferSale,
		Price:       dollars(a.PriceInfo.Amount),
		Address:     address(street, strings.TrimSpace(s.UnitValue), a.City, a.State, a.Zip),
		Beds:        intPtr(a.Beds),
		Baths:       a.Baths,
		Photos:      []string{},
		Coordinates: &listing.Coordinates{Lat: a.LatLong.Latitude, Lng: a.LatLong.Longitude},
		ObservedAt:  now,
		Raw:         raw,
	}
	if pid := string(a.AVM.PropertyID); pid != "" {
		l.SourceID = pid
	}
	if a.URL != "" {
		l.URL = origin + a.URL
	}
	if a.SqFt.Set {
		v := int(a.SqFt.V)
		l.SqFt = &v
	}
	if a.DaysOnMarket != nil {
		t := now.Add(-time.Duration(*a.DaysOnMarket) * day)
		l.ListedAt = &t
	}
	return l
}

func (p *Provider) lookupRental(ctx context.Context, u, id string, d detailItem, now time.Time) (listing.Listing, error) {
	b := geo.BoundsAround(listing.Coordinates{Lat: d.Address.LatLong.Latitude, Lng: d.Address.LatLong.Longitude}, rentalLookupMiles)
	zip := d.Address.Zip
	if !zipPattern.MatchString(zip) {
		zip = viewportAnchorZip
	}
	search := origin + "/zipcode/" + zip + "/apartments-for-rent/filter/" + fmt.Sprintf(rentalLookupFilter, b.North, b.South, b.East, b.West)
	items, err := p.Runner.Run(ctx, ActorID, actorInput{
		SearchURLs: []actorURL{{URL: search}},
		MaxResults: rentalLookupLimit,
	})
	if err != nil {
		return listing.Listing{}, err
	}
	q := listing.Query{Offer: listing.OfferRent}
	for _, raw := range items {
		var it item
		if err := json.Unmarshal(raw, &it); err != nil {
			return listing.Listing{}, fmt.Errorf("decode redfin item: %w", err)
		}
		if string(it.PropertyID) != id && strings.TrimSuffix(it.URL, "/") != u {
			continue
		}
		l, ok, err := toListing(raw, q, now)
		if err != nil {
			return listing.Listing{}, err
		}
		if ok && l.Offer == listing.OfferRent {
			return l, nil
		}
	}
	return listing.Listing{}, fmt.Errorf("redfin listing %s is not for sale or rent right now", u)
}
