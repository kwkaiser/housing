package zillow

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type zListing struct {
	Error          string        `json:"error"`
	ZPID           flexID        `json:"zpid"`
	PropertyURL    string        `json:"propertyUrl"`
	ListingStatus  string        `json:"listingStatus"`
	ListingPrice   *zPrice       `json:"listingPrice"`
	ListingAddress zAddress      `json:"listingAddress"`
	Coordinates    *zCoordinates `json:"coordinates"`
	Bedrooms       *float64      `json:"bedrooms"`
	Bathrooms      *float64      `json:"bathrooms"`
	LivingArea     *float64      `json:"livingArea"`
	ListingPhotos  []zPhoto      `json:"listingPhotos"`
	ScrapedAt      time.Time     `json:"scrapedAt"`
}

type zPhoto struct {
	URL string `json:"url"`
}

func (z zListing) base(fallbackOffer listing.OfferType, fallbackURL string, raw json.RawMessage) listing.Listing {
	return listing.Listing{
		Source:      listing.SourceZillow,
		SourceID:    string(z.ZPID),
		URL:         cmp.Or(z.PropertyURL, fallbackURL),
		Offer:       offer(z.ListingStatus, fallbackOffer),
		Address:     address(z.ListingAddress),
		Coordinates: z.Coordinates.toListing(),
		Beds:        intPtr(z.Bedrooms),
		Baths:       z.Bathrooms,
		SqFt:        intPtr(z.LivingArea),
		Photos:      z.photoURLs(),
		ObservedAt:  cmp.Or(z.ScrapedAt, time.Now().UTC()),
		Raw:         raw,
	}
}

func (z zListing) photoURLs() []string {
	out := make([]string, 0, len(z.ListingPhotos))
	for _, p := range z.ListingPhotos {
		if p.URL != "" {
			out = append(out, p.URL)
		}
	}
	return out
}

type zPrice struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type zAddress struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zipCode"`
	Full    string `json:"full"`
}

type zCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (c *zCoordinates) toListing() *listing.Coordinates {
	if c == nil {
		return nil
	}
	return &listing.Coordinates{Lat: c.Latitude, Lng: c.Longitude}
}

var unitPattern = regexp.MustCompile(`^(.*?)\s+(?:#|Apt\.?|Unit|Ste\.?)\s*(\S.*)$`)

func offer(status string, fallback listing.OfferType) listing.OfferType {
	switch status {
	case "forRent":
		return listing.OfferRent
	case "forSale":
		return listing.OfferSale
	}
	return fallback
}

func address(za zAddress) listing.Address {
	a := listing.Address{
		Formatted:  strings.Join(strings.Fields(za.Full), " "),
		Street:     strings.Join(strings.Fields(za.Street), " "),
		City:       za.City,
		State:      za.State,
		PostalCode: za.ZipCode,
	}
	if m := unitPattern.FindStringSubmatch(a.Street); m != nil {
		a.Street, a.Unit = m[1], m[2]
	}
	return a
}

func money(amount float64, currency string) listing.Money {
	if currency == "" {
		currency = "USD"
	}
	return listing.Money{Cents: int64(math.Round(amount * 100)), Currency: currency}
}

func parseDollars(s string) (int64, bool) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' {
			return r
		}
		return -1
	}, s)
	f, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		return 0, false
	}
	return int64(math.Round(f * 100)), true
}

func intPtr(f *float64) *int {
	if f == nil {
		return nil
	}
	v := int(*f)
	return &v
}

type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("zpid: %w", err)
	}
	*f = flexID(n.String())
	return nil
}

func mergeAmenities(base map[listing.Amenity]bool, extra ...map[listing.Amenity]bool) map[listing.Amenity]bool {
	out := maps.Clone(base)
	if out == nil {
		out = map[listing.Amenity]bool{}
	}
	for _, e := range extra {
		maps.Copy(out, e)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hasFold(ss []string, want string) bool {
	for _, s := range ss {
		if strings.EqualFold(strings.TrimSpace(s), want) {
			return true
		}
	}
	return false
}

func containsFold(ss []string, subs ...string) bool {
	for _, s := range ss {
		lower := strings.ToLower(s)
		for _, sub := range subs {
			if strings.Contains(lower, sub) {
				return true
			}
		}
	}
	return false
}
