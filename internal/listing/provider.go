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
