package zillow

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const day = 24 * time.Hour

var daysOnZillowBuckets = []struct {
	max   time.Duration
	value string
}{
	{1 * day, "1"},
	{7 * day, "7"},
	{14 * day, "14"},
	{30 * day, "30"},
	{90 * day, "90"},
	{183 * day, "6m"},
	{365 * day, "12m"},
	{730 * day, "24m"},
	{1095 * day, "36m"},
}

type filterValue struct {
	Value any `json:"value"`
}

type filterRange struct {
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

type mapBounds struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
}

type regionSelection struct {
	RegionID   int `json:"regionId"`
	RegionType int `json:"regionType"`
}

type searchQueryState struct {
	MapBounds       mapBounds         `json:"mapBounds"`
	RegionSelection []regionSelection `json:"regionSelection,omitempty"`
	FilterState     map[string]any    `json:"filterState"`
	IsMapVisible    bool              `json:"isMapVisible"`
	IsListVisible   bool              `json:"isListVisible"`
}

func buildSearchURL(q listing.Query, region Region) (string, error) {
	var selection []regionSelection
	var b geo.Bounds
	if q.Area.RadiusMiles > 0 {
		b = geo.BoundsAround(region.Center, q.Area.RadiusMiles)
	} else {
		code, ok := regionTypeCodes[region.Type]
		if !ok || region.ID == 0 {
			return "", fmt.Errorf("%w: %q is not a zillow region, set a radius", listing.ErrUnsupportedQuery, region.Name)
		}
		selection = []regionSelection{{RegionID: region.ID, RegionType: code}}
		b = geo.BoundsAround(region.Center, regionSpanMiles[region.Type])
	}

	filters := map[string]any{
		"sort": filterValue{"days"},
	}

	path := "for_sale"
	priceKey := "price"
	if q.Offer == listing.OfferRent {
		path = "for_rent"
		priceKey = "mp"
		filters["fr"] = filterValue{true}
		for _, k := range []string{"fsba", "fsbo", "nc", "cmsn", "auc", "fore"} {
			filters[k] = filterValue{false}
		}
	}

	if r := priceRange(q.MinPrice, q.MaxPrice); r != nil {
		filters[priceKey] = r
	}
	if q.MinBeds != nil || q.MaxBeds != nil {
		filters["beds"] = filterRange{Min: q.MinBeds, Max: q.MaxBeds}
	}
	if v, ok := daysOnZillow(q.MaxAge); ok {
		filters["doz"] = filterValue{v}
	}
	for _, a := range q.Amenities {
		filters[rentalAmenityFilters[a]] = filterValue{true}
	}

	state, err := json.Marshal(searchQueryState{
		MapBounds:       mapBounds{North: b.North, South: b.South, East: b.East, West: b.West},
		RegionSelection: selection,
		FilterState:     filters,
		IsMapVisible:    true,
		IsListVisible:   true,
	})
	if err != nil {
		return "", err
	}
	return "https://www.zillow.com/homes/" + path + "/?searchQueryState=" + url.QueryEscape(string(state)), nil
}

func priceRange(minPrice, maxPrice *listing.Money) *filterRange {
	if minPrice == nil && maxPrice == nil {
		return nil
	}
	r := &filterRange{}
	if minPrice != nil {
		v := int(minPrice.Cents / 100)
		r.Min = &v
	}
	if maxPrice != nil {
		v := int((maxPrice.Cents + 99) / 100)
		r.Max = &v
	}
	return r
}

func daysOnZillow(maxAge time.Duration) (string, bool) {
	if maxAge <= 0 {
		return "", false
	}
	for _, b := range daysOnZillowBuckets {
		if maxAge <= b.max {
			return b.value, true
		}
	}
	return "", false
}
