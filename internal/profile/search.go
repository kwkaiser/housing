package profile

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type Mode string

const (
	ModeRent Mode = "rent"
	ModeBuy  Mode = "buy"
)

func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case ModeRent, ModeBuy:
		return m, nil
	}
	return "", fmt.Errorf("invalid mode %q: use rent or buy", s)
}

func (m Mode) Offer() listing.OfferType {
	if m == ModeBuy {
		return listing.OfferSale
	}
	return listing.OfferRent
}

type Search struct {
	Location    string            `json:"location,omitempty"`
	RadiusMiles float64           `json:"radius_miles,omitempty"`
	MinPrice    *int              `json:"min_price,omitempty"`
	MaxPrice    *int              `json:"max_price,omitempty"`
	MinBeds     *int              `json:"min_beds,omitempty"`
	MaxBeds     *int              `json:"max_beds,omitempty"`
	MaxAgeDays  int               `json:"max_age_days,omitempty"`
	Amenities   []listing.Amenity `json:"amenities,omitempty"`
	Limit       int               `json:"limit,omitempty"`
}

func (s Search) Validate() error {
	var errs []error
	field := func(name, format string, args ...any) {
		errs = append(errs, FieldError{Field: name, Err: fmt.Errorf(format, args...)})
	}
	for _, a := range s.Amenities {
		if !slices.Contains(listing.Amenities, a) {
			field("amenities", "unknown amenity %q", a)
		}
	}
	if s.RadiusMiles < 0 {
		field("radius_miles", "radius must not be negative")
	}
	for _, n := range []struct {
		name, label string
		v           *int
	}{{"min_price", "min price", s.MinPrice}, {"max_price", "max price", s.MaxPrice}, {"min_beds", "min beds", s.MinBeds}, {"max_beds", "max beds", s.MaxBeds}} {
		if n.v != nil && *n.v < 0 {
			field(n.name, "%s must not be negative", n.label)
		}
	}
	if s.MinPrice != nil && s.MaxPrice != nil && *s.MinPrice > *s.MaxPrice {
		field("max_price", "min price %d is above max price %d", *s.MinPrice, *s.MaxPrice)
	}
	if s.MinBeds != nil && s.MaxBeds != nil && *s.MinBeds > *s.MaxBeds {
		field("max_beds", "min beds %d is above max beds %d", *s.MinBeds, *s.MaxBeds)
	}
	if s.MaxAgeDays < 0 {
		field("max_age_days", "max listing age must not be negative")
	}
	if s.Limit < 0 {
		field("limit", "result limit must not be negative")
	}
	return errors.Join(errs...)
}

func (s Search) Query(m Mode) listing.Query {
	q := listing.Query{
		Offer:     m.Offer(),
		Area:      listing.Area{Location: s.Location, RadiusMiles: s.RadiusMiles},
		MinBeds:   s.MinBeds,
		MaxBeds:   s.MaxBeds,
		MaxAge:    time.Duration(s.MaxAgeDays) * 24 * time.Hour,
		Amenities: s.Amenities,
		Limit:     s.Limit,
	}
	if s.MinPrice != nil {
		q.MinPrice = &listing.Money{Cents: int64(*s.MinPrice) * 100, Currency: "USD"}
	}
	if s.MaxPrice != nil {
		q.MaxPrice = &listing.Money{Cents: int64(*s.MaxPrice) * 100, Currency: "USD"}
	}
	return q
}
