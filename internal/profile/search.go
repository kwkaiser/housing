package profile

import (
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
	for _, a := range s.Amenities {
		if !slices.Contains(listing.Amenities, a) {
			return fmt.Errorf("unknown amenity %q", a)
		}
	}
	if s.MinPrice != nil && s.MaxPrice != nil && *s.MinPrice > *s.MaxPrice {
		return fmt.Errorf("min price %d is above max price %d", *s.MinPrice, *s.MaxPrice)
	}
	if s.MinBeds != nil && s.MaxBeds != nil && *s.MinBeds > *s.MaxBeds {
		return fmt.Errorf("min beds %d is above max beds %d", *s.MinBeds, *s.MaxBeds)
	}
	return nil
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
