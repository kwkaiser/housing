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
	SourceFacebook   Source = "facebook"
)

var Sources = []Source{SourceZillow, SourceRedfin, SourceRealtor, SourceCraigslist, SourceFacebook}

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
)

var Amenities = []Amenity{AmenityInUnitLaundry, AmenityDishwasher, AmenityParking, AmenityAirConditioning}

type Money struct {
	Cents    int64  `json:"cents"`
	Currency string `json:"currency"`
}

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Address struct {
	Formatted  string `json:"formatted"`
	Street     string `json:"street,omitempty"`
	Unit       string `json:"unit,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
}

type Listing struct {
	Source      Source                           `json:"source"`
	SourceID    string                           `json:"source_id"`
	URL         string                           `json:"url"`
	Offer       OfferType                        `json:"offer"`
	Price       Money                            `json:"price"`
	Address     Address                          `json:"address"`
	Coordinates *Coordinates                     `json:"coordinates,omitempty"`
	Beds        *int                             `json:"beds,omitempty"`
	Baths       *float64                         `json:"baths,omitempty"`
	SqFt        *int                             `json:"sqft,omitempty"`
	Amenities   map[Amenity]bool                 `json:"amenities,omitempty"`
	Description string                           `json:"description,omitempty"`
	Photos      []string                         `json:"photos"`
	Collages    []string                         `json:"collages,omitempty"`
	Assessments map[string]map[string]Assessment `json:"assessments,omitempty"`
	ListedAt    *time.Time                       `json:"listed_at,omitempty"`
	ObservedAt  time.Time                        `json:"observed_at"`
	Raw         json.RawMessage                  `json:"raw,omitempty"`
}
