package listing

import (
	"encoding/json"
	"time"
)

type Source string

const (
	SourceZillow     Source = "zillow"
	SourceRedfin     Source = "redfin"
	SourceRealtor    Source = "realtor"
	SourceCraigslist Source = "craigslist"
)

type OfferType string

const (
	OfferSale OfferType = "sale"
	OfferRent OfferType = "rent"
)

type Amenity string

const (
	AmenityInUnitLaundry   Amenity = "in_unit_laundry"
	AmenityDishwasher      Amenity = "dishwasher"
	AmenityParking         Amenity = "parking"
	AmenityAirConditioning Amenity = "air_conditioning"
	AmenityPetsAllowed     Amenity = "pets_allowed"
)

type Money struct {
	Cents    int64
	Currency string
}

type Coordinates struct {
	Lat float64
	Lng float64
}

type Address struct {
	Formatted  string
	Street     string
	Unit       string
	City       string
	State      string
	PostalCode string
}

type Listing struct {
	Source      Source
	SourceID    string
	URL         string
	Offer       OfferType
	Price       Money
	Address     Address
	Coordinates *Coordinates
	Beds        *int
	Baths       *float64
	SqFt        *int
	Amenities   map[Amenity]bool
	Photos      []string
	ListedAt    *time.Time
	ObservedAt  time.Time
	Raw         json.RawMessage
}
