package zillow

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const noResults = "No results found."

type item struct {
	Error          string        `json:"error"`
	ZPID           flexID        `json:"zpid"`
	ListingPrice   *zPrice       `json:"listingPrice"`
	ListingAddress zAddress      `json:"listingAddress"`
	Coordinates    *zCoordinates `json:"coordinates"`
	ListingStatus  string        `json:"listingStatus"`
	PropertyURL    string        `json:"propertyUrl"`
	CardType       string        `json:"cardType"`
	Bedrooms       *float64      `json:"bedrooms"`
	Bathrooms      *float64      `json:"bathrooms"`
	LivingArea     *float64      `json:"livingArea"`
	LivingAreaUnit string        `json:"livingAreaUnit"`
	DaysOnZillow   *int          `json:"daysOnZillow"`
	MainImage      string        `json:"mainImage"`
	ListingPhotos  []struct {
		URL string `json:"url"`
	} `json:"listingPhotos"`
	FactsAndFeatures struct {
		HasAirConditioning *bool `json:"hasAirConditioning"`
	} `json:"factsAndFeatures"`
	Units []struct {
		Price string `json:"price"`
		Beds  string `json:"beds"`
	} `json:"units"`
	ScrapedAt time.Time `json:"scrapedAt"`
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

func toListings(raw json.RawMessage, q listing.Query) ([]listing.Listing, error) {
	var it item
	if err := json.Unmarshal(raw, &it); err != nil {
		return nil, fmt.Errorf("decode zillow item: %w", err)
	}
	if it.Error != "" {
		if it.Error == noResults {
			return nil, nil
		}
		return nil, fmt.Errorf("zillow scraper: %s", it.Error)
	}
	if it.ZPID == "" || it.PropertyURL == "" {
		return nil, nil
	}

	base := listing.Listing{
		Source:     listing.SourceZillow,
		SourceID:   string(it.ZPID),
		URL:        it.PropertyURL,
		Offer:      offer(it.ListingStatus, q.Offer),
		Address:    address(it.ListingAddress),
		Photos:     photos(it),
		Amenities:  amenities(it, q),
		ObservedAt: it.ScrapedAt,
		Raw:        raw,
	}
	if base.ObservedAt.IsZero() {
		base.ObservedAt = time.Now().UTC()
	}
	base.Coordinates = it.Coordinates.toListing()

	if len(it.Units) > 0 {
		return unitListings(base, it), nil
	}

	if it.ListingPrice == nil {
		return nil, nil
	}
	l := base
	l.Price = money(it.ListingPrice.Amount, it.ListingPrice.Currency)
	l.Beds = intPtr(it.Bedrooms)
	l.Baths = it.Bathrooms
	if it.LivingAreaUnit == "" || it.LivingAreaUnit == "sqft" {
		l.SqFt = intPtr(it.LivingArea)
	}
	if it.DaysOnZillow != nil {
		t := l.ObservedAt.Add(-time.Duration(*it.DaysOnZillow) * day)
		l.ListedAt = &t
	}
	return []listing.Listing{l}, nil
}

func unitListings(base listing.Listing, it item) []listing.Listing {
	base.Address.Unit = ""
	var out []listing.Listing
	for i, u := range it.Units {
		cents, ok := parseDollars(u.Price)
		if !ok {
			continue
		}
		l := base
		l.SourceID = fmt.Sprintf("%s#%d", it.ZPID, i)
		l.Price = listing.Money{Cents: cents, Currency: "USD"}
		if beds, err := strconv.Atoi(u.Beds); err == nil {
			l.Beds = &beds
		}
		out = append(out, l)
	}
	return out
}

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

func photos(it item) []string {
	out := make([]string, 0, len(it.ListingPhotos))
	for _, p := range it.ListingPhotos {
		if p.URL != "" {
			out = append(out, p.URL)
		}
	}
	if len(out) == 0 && it.MainImage != "" {
		out = append(out, it.MainImage)
	}
	return out
}

func amenities(it item, q listing.Query) map[listing.Amenity]bool {
	out := map[listing.Amenity]bool{}
	for _, a := range q.Amenities {
		out[a] = true
	}
	if ac := it.FactsAndFeatures.HasAirConditioning; ac != nil && *ac {
		out[listing.AmenityAirConditioning] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
