package listing

import (
	"context"
	"errors"
	"time"
)

var ErrUnsupportedQuery = errors.New("query not supported by provider")

type Area struct {
	Location    string
	RadiusMiles float64
}

type Query struct {
	Offer     OfferType
	Area      Area
	MinPrice  *Money
	MaxPrice  *Money
	MinBeds   *int
	MaxBeds   *int
	MaxAge    time.Duration
	Amenities []Amenity
	Limit     int
}

type Provider interface {
	Source() Source
	SupportedAmenities(offer OfferType) []Amenity
	Search(ctx context.Context, q Query) ([]Listing, error)
}

type Enricher interface {
	Enrich(ctx context.Context, listings []Listing) ([]Listing, error)
}

func (q Query) Matches(l Listing) bool {
	if q.Offer != "" && l.Offer != q.Offer {
		return false
	}
	if q.MinPrice != nil && l.Price.Cents < q.MinPrice.Cents {
		return false
	}
	if q.MaxPrice != nil && l.Price.Cents > q.MaxPrice.Cents {
		return false
	}
	if q.MinBeds != nil && (l.Beds == nil || *l.Beds < *q.MinBeds) {
		return false
	}
	if q.MaxBeds != nil && (l.Beds == nil || *l.Beds > *q.MaxBeds) {
		return false
	}
	for _, a := range q.Amenities {
		if !l.Amenities[a] {
			return false
		}
	}
	return true
}
