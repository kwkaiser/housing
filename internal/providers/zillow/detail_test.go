package zillow

import (
	"context"
	"encoding/json"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const (
	homeURL     = "https://www.zillow.com/homedetails/504-W-35th-St-Austin-TX-78705/29398895_zpid/"
	buildingURL = "https://www.zillow.com/apartments/austin-tx/cortland-bluff-springs/CjmTsL/"
)

func TestEnrich(t *testing.T) {
	items := append(loadFixture(t, "detail_home.json"), loadFixture(t, "detail_building.json")...)
	runner := &fakeRunner{items: items}
	p := New(runner, fakeRegions{austin})

	in := []listing.Listing{
		{Source: listing.SourceZillow, SourceID: "29398895", URL: homeURL, Offer: listing.OfferRent, Beds: ptr(6)},
		{Source: listing.SourceZillow, SourceID: "30.181015--97.77314#0", URL: buildingURL, Offer: listing.OfferRent, Beds: ptr(2),
			Amenities: map[listing.Amenity]bool{listing.AmenityInUnitLaundry: true}},
		{Source: listing.SourceZillow, SourceID: "30.181015--97.77314#1", URL: buildingURL, Offer: listing.OfferRent, Beds: ptr(3)},
		{Source: listing.SourceCraigslist, SourceID: "cl", URL: "https://example.com"},
	}

	got, err := p.Enrich(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}

	input := runner.input.(detailInput)
	if len(input.StartURLs) != 2 || input.PropertyStatus != "FOR_RENT" || input.ExtractBuildingUnits != "for_rent" {
		t.Errorf("detail input = %+v", input)
	}

	if len(got) != 1+32+1 {
		t.Fatalf("got %d listings, want home + 32 units + passthrough", len(got))
	}

	home := got[0]
	if *home.Baths != 3 || *home.SqFt != 2436 || home.ListedAt == nil || home.ListedAt.Format("2006-01-02") != "2026-09-29" {
		t.Errorf("home = %+v", home)
	}
	for _, a := range []listing.Amenity{listing.AmenityDishwasher, listing.AmenityInUnitLaundry, listing.AmenityAirConditioning, listing.AmenityParking} {
		if !home.Amenities[a] {
			t.Errorf("home missing %s", a)
		}
	}

	unit := got[1]
	if unit.SourceID != "2053141628" || unit.Address.Unit != "6316" || unit.Price.Cents != 141900 ||
		*unit.Beds != 1 || *unit.Baths != 1 || *unit.SqFt != 750 {
		t.Errorf("unit = %+v", unit)
	}
	for _, a := range []listing.Amenity{listing.AmenityDishwasher, listing.AmenityInUnitLaundry, listing.AmenityAirConditioning, listing.AmenityParking} {
		if !unit.Amenities[a] {
			t.Errorf("unit missing %s", a)
		}
	}

	var raw struct {
		BuildingZPID string `json:"buildingZpid"`
		FloorPlan    struct {
			Name string `json:"name"`
		} `json:"floorPlan"`
		Unit struct {
			UnitNumber string `json:"unitNumber"`
		} `json:"unit"`
	}
	if err := json.Unmarshal(unit.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.BuildingZPID != "342716910" || raw.FloorPlan.Name != "SoCo" || raw.Unit.UnitNumber != "Unit 6316" {
		t.Errorf("unit raw = %s", unit.Raw)
	}

	ids := map[string]bool{}
	for _, l := range got[1:33] {
		if ids[l.SourceID] {
			t.Errorf("duplicate unit %s", l.SourceID)
		}
		ids[l.SourceID] = true
		if l.URL != buildingURL || l.Beds == nil || l.Baths == nil || l.SqFt == nil {
			t.Errorf("incomplete unit %+v", l)
		}
	}

	if got[33].Source != listing.SourceCraigslist {
		t.Error("non-zillow listings should pass through")
	}
}

func TestDishwasherIsNotLaundry(t *testing.T) {
	var it detailItem
	it.PropertyFeatures.Appliances = []string{"Dishwasher", "Dryer"}
	got := homeAmenities(it)
	if got[listing.AmenityInUnitLaundry] || !got[listing.AmenityDishwasher] {
		t.Errorf("got %v", got)
	}
}
