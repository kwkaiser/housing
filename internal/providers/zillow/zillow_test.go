package zillow

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type fakeRunner struct {
	items []json.RawMessage
	input any
}

func (f *fakeRunner) Run(_ context.Context, _ string, input any) ([]json.RawMessage, error) {
	f.input = input
	return f.items, nil
}

type fakeRegions struct{ region Region }

func (f fakeRegions) Resolve(context.Context, string) (Region, error) {
	return f.region, nil
}

var austin = Region{Name: "Austin, TX", ID: 10221, Type: "city", Center: listing.Coordinates{Lat: 30.28, Lng: -97.79}}

func loadFixture(t *testing.T, name string) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func decodeState(t *testing.T, rawURL string) map[string]any {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal([]byte(u.Query().Get("searchQueryState")), &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func ptr[T any](v T) *T { return &v }

func TestBuildSearchURLRent(t *testing.T) {
	q := listing.Query{
		Offer:     listing.OfferRent,
		MinPrice:  &listing.Money{Cents: 100000},
		MaxPrice:  &listing.Money{Cents: 250050},
		MinBeds:   ptr(1),
		MaxAge:    10 * day,
		Amenities: []listing.Amenity{listing.AmenityInUnitLaundry, listing.AmenityParking},
	}
	u, err := buildSearchURL(q, austin)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://www.zillow.com/homes/for_rent/") {
		t.Fatalf("unexpected path: %s", u)
	}

	state := decodeState(t, u)
	fs := state["filterState"].(map[string]any)
	checks := map[string]any{
		"fr":    map[string]any{"value": true},
		"fsba":  map[string]any{"value": false},
		"mp":    map[string]any{"min": 1000.0, "max": 2501.0},
		"beds":  map[string]any{"min": 1.0},
		"doz":   map[string]any{"value": "14"},
		"lau":   map[string]any{"value": true},
		"parka": map[string]any{"value": true},
	}
	for k, want := range checks {
		gotJSON, _ := json.Marshal(fs[k])
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("filter %s = %s, want %s", k, gotJSON, wantJSON)
		}
	}
	sel := state["regionSelection"].([]any)[0].(map[string]any)
	if sel["regionId"] != 10221.0 || sel["regionType"] != 6.0 {
		t.Errorf("regionSelection = %v", sel)
	}
}

func TestBuildSearchURLRadius(t *testing.T) {
	q := listing.Query{Offer: listing.OfferSale, Area: listing.Area{RadiusMiles: 5}, MaxPrice: &listing.Money{Cents: 50000000}}
	u, err := buildSearchURL(q, Region{Name: "somewhere", Center: austin.Center})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://www.zillow.com/homes/for_sale/") {
		t.Fatalf("unexpected path: %s", u)
	}
	state := decodeState(t, u)
	if _, ok := state["regionSelection"]; ok {
		t.Error("radius search should not select a region")
	}
	fs := state["filterState"].(map[string]any)
	if _, ok := fs["price"]; !ok {
		t.Error("sale search should filter on price")
	}
	if _, ok := fs["fr"]; ok {
		t.Error("sale search should not set for-rent filter")
	}
}

func TestBuildSearchURLNonRegionWithoutRadius(t *testing.T) {
	_, err := buildSearchURL(listing.Query{Offer: listing.OfferSale}, Region{Name: "123 Main St"})
	if !errors.Is(err, listing.ErrUnsupportedQuery) {
		t.Fatalf("got %v", err)
	}
}

func TestDaysOnZillow(t *testing.T) {
	cases := map[time.Duration]string{
		time.Hour:  "1",
		7 * day:    "7",
		8 * day:    "14",
		100 * day:  "6m",
		1095 * day: "36m",
	}
	for age, want := range cases {
		if got, ok := daysOnZillow(age); !ok || got != want {
			t.Errorf("daysOnZillow(%v) = %q, %v; want %q", age, got, ok, want)
		}
	}
	if _, ok := daysOnZillow(0); ok {
		t.Error("zero age should not filter")
	}
	if _, ok := daysOnZillow(2000 * day); ok {
		t.Error("age beyond largest bucket should not filter")
	}
}

func TestSearchMapsRentals(t *testing.T) {
	runner := &fakeRunner{items: loadFixture(t, "rent.json")}
	p := New(runner, fakeRegions{austin})

	got, err := p.Search(context.Background(), listing.Query{
		Offer:     listing.OfferRent,
		Area:      listing.Area{Location: "Austin, TX"},
		Amenities: []listing.Amenity{listing.AmenityInUnitLaundry},
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d listings, want 1 home + 3 units", len(got))
	}

	home := got[0]
	if home.SourceID != "29398895" || home.Price.Cents != 645000 || *home.Beds != 6 || *home.Baths != 3 || *home.SqFt != 2436 {
		t.Errorf("home = %+v", home)
	}
	if home.ListedAt == nil || !home.ListedAt.Equal(home.ObservedAt) {
		t.Errorf("home listed at %v", home.ListedAt)
	}
	if !home.Amenities[listing.AmenityInUnitLaundry] {
		t.Error("requested amenity should be recorded")
	}
	if _, ok := home.Amenities[listing.AmenityAirConditioning]; ok {
		t.Error("hasAirConditioning=false should be treated as unknown")
	}

	unit := got[1]
	if unit.SourceID != "30.255857--97.76304#0" || unit.Price.Cents != 114900 || *unit.Beds != 0 {
		t.Errorf("unit = %+v", unit)
	}
	if unit.Address.Street != "1100 S Lamar Blvd" || unit.Address.Unit != "" {
		t.Errorf("unit address = %+v", unit.Address)
	}
	if unit.Baths != nil || unit.SqFt != nil || unit.ListedAt != nil {
		t.Errorf("building units should not report baths/sqft/listed_at: %+v", unit)
	}

	input := runner.input.(actorInput)
	if input.ResultsLimit != 10 || len(input.SearchURLs) != 1 {
		t.Errorf("actor input = %+v", input)
	}
}

func TestSearchPostFilters(t *testing.T) {
	p := New(&fakeRunner{items: loadFixture(t, "rent.json")}, fakeRegions{austin})
	got, err := p.Search(context.Background(), listing.Query{
		Offer:    listing.OfferRent,
		Area:     listing.Area{Location: "Austin, TX"},
		MaxPrice: &listing.Money{Cents: 250000},
		MinBeds:  ptr(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got {
		if l.Price.Cents > 250000 || *l.Beds < 1 {
			t.Errorf("listing escaped post-filter: %s %d %d", l.SourceID, l.Price.Cents, *l.Beds)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d listings, want only the 1bd unit", len(got))
	}
}

func TestSearchMapsSales(t *testing.T) {
	p := New(&fakeRunner{items: loadFixture(t, "sale.json")}, fakeRegions{austin})
	got, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "Austin, TX"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d listings", len(got))
	}
	for _, l := range got {
		if l.Offer != listing.OfferSale || l.Price.Currency != "USD" || len(l.Photos) == 0 {
			t.Errorf("sale = %+v", l)
		}
	}
	if got[1].Coordinates != nil {
		t.Error("listing without coordinates should leave them nil")
	}
}

func TestSearchRejectsUnsupportedAmenity(t *testing.T) {
	p := New(&fakeRunner{}, fakeRegions{austin})
	cases := []listing.Query{
		{Offer: listing.OfferRent, Area: listing.Area{Location: "x"}, Amenities: []listing.Amenity{listing.AmenityDishwasher}},
		{Offer: listing.OfferSale, Area: listing.Area{Location: "x"}, Amenities: []listing.Amenity{listing.AmenityParking}},
	}
	for _, q := range cases {
		if _, err := p.Search(context.Background(), q); !errors.Is(err, listing.ErrUnsupportedQuery) {
			t.Errorf("%v: got %v", q, err)
		}
	}
}

func TestNoResultsItem(t *testing.T) {
	p := New(&fakeRunner{items: []json.RawMessage{json.RawMessage(`{"error":"No results found."}`)}}, fakeRegions{austin})
	got, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "x"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
