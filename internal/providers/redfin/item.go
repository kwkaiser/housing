package redfin

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type value[T any] struct {
	V   T
	Set bool
}

func (v *value[T]) UnmarshalJSON(b []byte) error {
	var wrapped struct {
		Value *T `json:"value"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil && wrapped.Value != nil {
		v.V, v.Set = *wrapped.Value, true
		return nil
	}
	var plain T
	if err := json.Unmarshal(b, &plain); err == nil {
		v.V, v.Set = plain, true
	}
	return nil
}

type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("id: %w", err)
	}
	*f = flexString(n.String())
	return nil
}

type fact struct {
	Description string `json:"description"`
}

type coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type item struct {
	OfferType  string     `json:"offerType"`
	URL        string     `json:"url"`
	PropertyID flexString `json:"propertyId"`
	KeyFacts   []fact     `json:"keyFacts"`

	Price        value[float64]     `json:"price"`
	Beds         *float64           `json:"beds"`
	Baths        *float64           `json:"baths"`
	SqFt         value[float64]     `json:"sqFt"`
	DOM          value[int]         `json:"dom"`
	StreetLine   value[string]      `json:"streetLine"`
	UnitNumber   value[string]      `json:"unitNumber"`
	City         string             `json:"city"`
	State        string             `json:"state"`
	Zip          value[string]      `json:"zip"`
	LatLong      value[coordinates] `json:"latLong"`
	MLSID        value[string]      `json:"mlsId"`
	DataSourceID int                `json:"dataSourceId"`
	Photos       value[string]      `json:"photos"`
	Remarks      string             `json:"listingRemarks"`
	Tags         []string           `json:"listingTags"`

	RentalID    string `json:"rentalId"`
	Description string `json:"description"`
	AddressInfo struct {
		Centroid struct {
			Centroid coordinates `json:"centroid"`
		} `json:"centroid"`
		Street string `json:"formattedStreetLine"`
		City   string `json:"city"`
		State  string `json:"state"`
		Zip    string `json:"zip"`
	} `json:"addressInfo"`
	RentPriceRange numberRange `json:"rentPriceRange"`
	BedRange       numberRange `json:"bedRange"`
	BathRange      numberRange `json:"bathRange"`
	SqftRange      numberRange `json:"sqftRange"`
	PhotosInfo     struct {
		PhotoRanges []struct {
			Start   int        `json:"startPos"`
			End     int        `json:"endPos"`
			Version flexString `json:"version"`
		} `json:"photoRanges"`
	} `json:"photosInfo"`
}

type numberRange struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

const photoBase = "https://ssl.cdn-redfin.com/photo"

func toListing(raw json.RawMessage, q listing.Query, now time.Time) (listing.Listing, bool, error) {
	var it item
	if err := json.Unmarshal(raw, &it); err != nil {
		return listing.Listing{}, false, fmt.Errorf("decode redfin item: %w", err)
	}
	if it.URL == "" {
		return listing.Listing{}, false, nil
	}
	if it.OfferType == "rent" || it.RentalID != "" {
		return rental(it, raw, q, now)
	}
	return sale(it, raw, now)
}

func sale(it item, raw json.RawMessage, now time.Time) (listing.Listing, bool, error) {
	if !it.Price.Set || it.PropertyID == "" {
		return listing.Listing{}, false, nil
	}
	street := strings.TrimSpace(it.StreetLine.V)
	unit := strings.TrimSpace(strings.TrimPrefix(it.UnitNumber.V, "#"))
	if unit != "" {
		street = strings.TrimSpace(strings.TrimSuffix(street, it.UnitNumber.V))
	}
	l := listing.Listing{
		Source:      listing.SourceRedfin,
		SourceID:    string(it.PropertyID),
		URL:         it.URL,
		Offer:       listing.OfferSale,
		Price:       dollars(it.Price.V),
		Address:     address(street, unit, it.City, it.State, it.Zip.V),
		Beds:        intPtr(it.Beds),
		Baths:       it.Baths,
		Description: it.Remarks,
		Photos:      salePhotos(it),
		ObservedAt:  now,
		Raw:         raw,
	}
	if it.LatLong.Set {
		l.Coordinates = &listing.Coordinates{Lat: it.LatLong.V.Latitude, Lng: it.LatLong.V.Longitude}
	}
	if it.SqFt.Set {
		v := int(it.SqFt.V)
		l.SqFt = &v
	}
	if it.DOM.Set {
		t := now.Add(-time.Duration(it.DOM.V) * day)
		l.ListedAt = &t
	}
	l.Amenities = amenities(it.Remarks, it.Tags, it.KeyFacts)
	return l, true, nil
}

func rental(it item, raw json.RawMessage, q listing.Query, now time.Time) (listing.Listing, bool, error) {
	if it.RentPriceRange.Min == nil {
		return listing.Listing{}, false, nil
	}
	a := it.AddressInfo
	id := it.RentalID
	if id == "" {
		id = string(it.PropertyID)
	}
	l := listing.Listing{
		Source:      listing.SourceRedfin,
		SourceID:    id,
		URL:         it.URL,
		Offer:       listing.OfferRent,
		Price:       dollars(*it.RentPriceRange.Min),
		Address:     address(a.Street, "", a.City, a.State, a.Zip),
		Beds:        bedsFor(it.BedRange, q.MinBeds),
		Baths:       it.BathRange.Min,
		SqFt:        intPtr(it.SqftRange.Min),
		Description: it.Description,
		Photos:      rentalPhotos(it),
		ObservedAt:  now,
		Raw:         raw,
	}
	if c := a.Centroid.Centroid; c.Latitude != 0 || c.Longitude != 0 {
		l.Coordinates = &listing.Coordinates{Lat: c.Latitude, Lng: c.Longitude}
	}
	l.Amenities = amenities(it.Description, nil, it.KeyFacts)
	return l, true, nil
}

func bedsFor(r numberRange, minBeds *int) *int {
	if r.Min == nil {
		return nil
	}
	beds := int(*r.Min)
	if minBeds != nil && beds < *minBeds && r.Max != nil && int(*r.Max) >= *minBeds {
		beds = *minBeds
	}
	return &beds
}

func salePhotos(it item) []string {
	mls := it.MLSID.V
	if !it.Photos.Set || mls == "" || it.DataSourceID == 0 {
		return []string{}
	}
	dir := mls
	if len(dir) > 3 {
		dir = dir[len(dir)-3:]
	}
	var out []string
	for _, r := range strings.Split(it.Photos.V, ",") {
		span, version, ok := strings.Cut(strings.TrimSpace(r), ":")
		if !ok {
			continue
		}
		start, end, ok := parseSpan(span)
		if !ok {
			continue
		}
		for i := start; i <= end; i++ {
			name := fmt.Sprintf("%s_%d_%s.jpg", mls, i, version)
			if i == 0 && version == "0" {
				name = mls + "_0.jpg"
			}
			out = append(out, fmt.Sprintf("%s/%d/bigphoto/%s/%s", photoBase, it.DataSourceID, dir, name))
		}
	}
	return out
}

func rentalPhotos(it item) []string {
	out := []string{}
	if it.RentalID == "" {
		return out
	}
	for _, r := range it.PhotosInfo.PhotoRanges {
		for i := r.Start; i <= r.End; i++ {
			out = append(out, fmt.Sprintf("%s/rent/%s/bigphoto/%d_%s.jpg", photoBase, it.RentalID, i, r.Version))
		}
	}
	return out
}

func parseSpan(s string) (int, int, bool) {
	a, b, ok := strings.Cut(s, "-")
	start, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	if !ok {
		return start, start, true
	}
	end, err := strconv.Atoi(b)
	if err != nil || end < start {
		return 0, 0, false
	}
	return start, end, true
}

func amenities(text string, tags []string, facts []fact) map[listing.Amenity]bool {
	parts := []string{text}
	parts = append(parts, tags...)
	for _, f := range facts {
		parts = append(parts, f.Description)
	}
	found := listing.AmenitiesFromText(strings.Join(parts, ". "))
	if len(found) == 0 {
		return nil
	}
	return found
}

func address(street, unit, city, state, zip string) listing.Address {
	formatted := street
	if unit != "" {
		formatted += " #" + unit
	}
	for _, part := range []string{city, strings.TrimSpace(state + " " + zip)} {
		if part != "" {
			formatted += ", " + part
		}
	}
	return listing.Address{Formatted: formatted, Street: street, Unit: unit, City: city, State: state, PostalCode: zip}
}

func dollars(v float64) listing.Money {
	return listing.Money{Cents: int64(math.Round(v * 100)), Currency: "USD"}
}

func intPtr(f *float64) *int {
	if f == nil {
		return nil
	}
	v := int(*f)
	return &v
}
