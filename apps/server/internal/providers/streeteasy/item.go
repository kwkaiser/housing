package streeteasy

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type item struct {
	ID          string  `json:"id"`
	URLPath     string  `json:"urlPath"`
	Price       float64 `json:"price"`
	SaleType    *string `json:"saleType"`
	Description string  `json:"description"`
	OnMarketAt  string  `json:"onMarketAt"`
	Street      string  `json:"street"`
	Unit        string  `json:"unit"`
	ZipCode     string  `json:"zipCode"`
	LivingArea  int     `json:"livingAreaSize"`
	GeoPoint    *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"geoPoint"`
	Pricing struct {
		Price float64 `json:"price"`
	} `json:"pricing"`
	Images []string `json:"images"`
	Photos []struct {
		URL string `json:"url"`
	} `json:"photos"`
	Details struct {
		Address struct {
			Street      string `json:"street"`
			City        string `json:"city"`
			State       string `json:"state"`
			ZipCode     string `json:"zipCode"`
			DisplayUnit string `json:"displayUnit"`
		} `json:"address"`
		Beds       *int     `json:"bedroomCount"`
		FullBaths  *float64 `json:"fullBathroomCount"`
		HalfBaths  *float64 `json:"halfBathroomCount"`
		LivingArea *int     `json:"livingAreaSize"`
		Amenities  struct {
			List         []string `json:"list"`
			ParkingTypes []string `json:"parkingTypes"`
		} `json:"amenities"`
		Features struct {
			List []string `json:"list"`
		} `json:"features"`
	} `json:"propertyDetails"`
}

var featureAmenities = map[string]listing.Amenity{
	"WASHER_DRYER": listing.AmenityInUnitLaundry,
	"DISHWASHER":   listing.AmenityDishwasher,
	"CENTRAL_AC":   listing.AmenityAirConditioning,
	"PARKING":      listing.AmenityParking,
}

func toListing(raw json.RawMessage, offer listing.OfferType, now time.Time) (listing.Listing, bool, error) {
	var it item
	if err := json.Unmarshal(raw, &it); err != nil {
		return listing.Listing{}, false, fmt.Errorf("decode streeteasy item: %w", err)
	}
	price := it.Pricing.Price
	if price == 0 {
		price = it.Price
	}
	if it.ID == "" || it.URLPath == "" || price <= 0 {
		return listing.Listing{}, false, nil
	}
	if offer == "" {
		offer = listing.OfferRent
		if it.SaleType != nil {
			offer = listing.OfferSale
		}
	}

	l := listing.Listing{
		Source:      listing.SourceStreetEasy,
		SourceID:    it.ID,
		URL:         origin + it.URLPath,
		Offer:       offer,
		Price:       listing.Money{Cents: int64(math.Round(price * 100)), Currency: "USD"},
		Address:     it.address(),
		Beds:        it.Details.Beds,
		Baths:       it.baths(),
		Description: strings.TrimSpace(it.Description),
		Photos:      it.photos(),
		Amenities:   it.amenities(),
		ObservedAt:  now,
		Raw:         raw,
	}
	if g := it.GeoPoint; g != nil && (g.Latitude != 0 || g.Longitude != 0) {
		l.Coordinates = &listing.Coordinates{Lat: g.Latitude, Lng: g.Longitude}
	}
	if a := it.Details.LivingArea; a != nil && *a > 0 {
		l.SqFt = a
	} else if it.LivingArea > 0 {
		l.SqFt = &it.LivingArea
	}
	if t, err := time.Parse(time.DateOnly, it.OnMarketAt); err == nil {
		l.ListedAt = &t
	}
	return l, true, nil
}

func (it item) address() listing.Address {
	a := it.Details.Address
	street := firstNonEmpty(a.Street, it.Street)
	unit := strings.TrimSpace(strings.TrimPrefix(firstNonEmpty(a.DisplayUnit, it.Unit), "#"))
	city := titleCase(a.City)
	zip := firstNonEmpty(a.ZipCode, it.ZipCode)

	formatted := street
	if unit != "" {
		formatted += " #" + unit
	}
	for _, part := range []string{city, strings.TrimSpace(a.State + " " + zip)} {
		if part != "" {
			formatted += ", " + part
		}
	}
	return listing.Address{Formatted: formatted, Street: street, Unit: unit, City: city, State: a.State, PostalCode: zip}
}

func (it item) baths() *float64 {
	if it.Details.FullBaths == nil {
		return nil
	}
	v := *it.Details.FullBaths
	if it.Details.HalfBaths != nil {
		v += *it.Details.HalfBaths / 2
	}
	return &v
}

func (it item) photos() []string {
	if len(it.Images) > 0 {
		return it.Images
	}
	out := []string{}
	for _, p := range it.Photos {
		if p.URL != "" {
			out = append(out, p.URL)
		}
	}
	return out
}

func (it item) amenities() map[listing.Amenity]bool {
	found := listing.AmenitiesFromText(it.Description)
	if found == nil {
		found = map[listing.Amenity]bool{}
	}
	for _, tag := range append(it.Details.Features.List, it.Details.Amenities.List...) {
		if a, ok := featureAmenities[tag]; ok {
			found[a] = true
		}
	}
	if len(it.Details.Amenities.ParkingTypes) > 0 {
		found[listing.AmenityParking] = true
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
