package zillow

import (
	"encoding/json"
	"strings"
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

	got, err := p.Enrich(t.Context(), in)
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

func TestLookup(t *testing.T) {
	runner := &fakeRunner{items: loadFixture(t, "detail_offmarket.json")}
	p := New(runner, fakeRegions{austin})

	l, err := p.Lookup(t.Context(), "https://www.zillow.com/homedetails/7-Adams-St-APT-4-Somerville-MA-02145/71142320_zpid/?utm=x#photos")
	if err != nil {
		t.Fatal(err)
	}
	input := runner.input.(detailInput)
	if got := input.StartURLs[0].URL; got != "https://www.zillow.com/homedetails/7-Adams-St-APT-4-Somerville-MA-02145/71142320_zpid/" {
		t.Errorf("lookup should strip query and fragment, got %s", got)
	}
	if l.SourceID != "71142320" || l.Offer != "" || *l.Beds != 2 || *l.Baths != 1 || *l.SqFt != 872 {
		t.Errorf("listing = %+v", l)
	}
	if !strings.Contains(l.Description, "skylights") || len(l.Photos) != 3 || l.Address.Formatted == "" {
		t.Errorf("listing missing description/photos/address: %+v", l)
	}
	if !l.Amenities[listing.AmenityDishwasher] {
		t.Errorf("amenities = %v", l.Amenities)
	}
}

func TestLookupRejectsOtherHosts(t *testing.T) {
	p := New(&fakeRunner{}, fakeRegions{austin})
	if _, err := p.Lookup(t.Context(), "https://www.redfin.com/x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnrichSmallBuildingUnits(t *testing.T) {
	const building = "https://www.zillow.com/b/98-perkins-st-somerville-ma-5Xm2RF/"
	p := New(&fakeRunner{items: loadFixture(t, "detail_small_building.json")}, fakeRegions{austin})
	in := []listing.Listing{
		{Source: listing.SourceZillow, SourceID: "42.38487--71.08078#0", URL: building, Offer: listing.OfferRent,
			Amenities: map[listing.Amenity]bool{listing.AmenityInUnitLaundry: true}},
		{Source: listing.SourceZillow, SourceID: "42.38487--71.08078#1", URL: building, Offer: listing.OfferRent},
	}
	got, err := p.Enrich(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d listings, want one per unit", len(got))
	}
	ids := map[string]bool{}
	for _, l := range got {
		ids[l.SourceID] = true
		if strings.Contains(l.SourceID, "#") || l.Description == "" || l.Price.Cents == 0 || l.Beds == nil || !strings.Contains(l.URL, "/homedetails/") {
			t.Errorf("unit not enriched: %+v", l)
		}
		if !l.Amenities[listing.AmenityInUnitLaundry] {
			t.Error("amenities guaranteed by the search should carry over to units")
		}
	}
	if !ids["464622964"] || !ids["465235055"] {
		t.Errorf("unit ids = %v", ids)
	}
}
