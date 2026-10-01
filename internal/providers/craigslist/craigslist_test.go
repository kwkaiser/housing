package craigslist

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

type fakeRunner struct {
	items []json.RawMessage
	input actorInput
}

func (f *fakeRunner) Run(_ context.Context, _ string, input any) ([]json.RawMessage, error) {
	f.input = input.(actorInput)
	return f.items, nil
}

type fakeRegions struct{ region zillow.Region }

func (f fakeRegions) Resolve(context.Context, string) (zillow.Region, error) { return f.region, nil }

var (
	zip02144 = zillow.Region{Name: "02144", Type: "zipcode", Center: listing.Coordinates{Lat: 42.3995, Lng: -71.1223}}
	now      = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
)

func ptr[T any](v T) *T { return &v }

func fixture(t *testing.T) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/rent.json")
	if err != nil {
		t.Fatal(err)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func newProvider(items []json.RawMessage, region zillow.Region) (*Provider, *fakeRunner) {
	r := &fakeRunner{items: items}
	p := New(r, fakeRegions{region})
	p.Now = func() time.Time { return now }
	return p, r
}

func TestBuildSearchURL(t *testing.T) {
	q := listing.Query{
		Offer:     listing.OfferRent,
		MinPrice:  &listing.Money{Cents: 150000},
		MaxPrice:  &listing.Money{Cents: 400000},
		MinBeds:   ptr(1),
		MaxBeds:   ptr(3),
		MaxAge:    12 * time.Hour,
		Amenities: []listing.Amenity{listing.AmenityInUnitLaundry, listing.AmenityParking},
	}
	got := buildSearchURL(q, searchArea{subdomain: "boston", postal: "02144", radius: 2})
	want := "https://boston.craigslist.org/search/apa?sort=date&hasPic=1&min_price=1500&max_price=4000&min_bedrooms=1&max_bedrooms=3&postedToday=1" +
		"&laundry=1&parking=1&parking=2&parking=3&parking=4&parking=6&postal=02144&search_distance=2"
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}

	near := buildSearchURL(listing.Query{Offer: listing.OfferRent}, searchArea{subdomain: "boston", center: listing.Coordinates{Lat: 42.38765, Lng: -71.0995}, radius: 1.5})
	if near != "https://boston.craigslist.org/search/apa?sort=date&hasPic=1&lat=42.3877&lon=-71.0995&search_distance=1.5" {
		t.Errorf("lat/lon url = %s", near)
	}
}

func TestResolve(t *testing.T) {
	p, _ := newProvider(nil, zip02144)
	a, err := p.resolve(context.Background(), listing.Area{Location: "02144"})
	if err != nil || a.subdomain != "boston" || a.postal != "02144" || a.radius != 2 || a.exact {
		t.Errorf("zip area = %+v %v", a, err)
	}
	a, err = p.resolve(context.Background(), listing.Area{Location: "Somerville, MA", RadiusMiles: 1})
	if err != nil || a.postal != "" || a.radius != 1 || !a.exact {
		t.Errorf("radius area = %+v %v", a, err)
	}

	far, _ := newProvider(nil, zillow.Region{Name: "Nowhere", Type: "city", Center: listing.Coordinates{Lat: 44.0, Lng: -110.0}})
	if _, err := far.resolve(context.Background(), listing.Area{Location: "Nowhere"}); !errors.Is(err, listing.ErrUnsupportedQuery) {
		t.Errorf("location far from any region should be rejected: %v", err)
	}
	state, _ := newProvider(nil, zillow.Region{Name: "MA", Type: "state", Center: listing.Coordinates{Lat: 42.3, Lng: -71.8}})
	if _, err := state.resolve(context.Background(), listing.Area{Location: "MA"}); !errors.Is(err, listing.ErrUnsupportedQuery) {
		t.Errorf("state without radius should be rejected: %v", err)
	}
}

func TestSearch(t *testing.T) {
	p, r := newProvider(fixture(t), zip02144)
	got, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "02144"}, MinBeds: ptr(1), Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.MaxItems != 5 || !r.input.IncludeDetails || !strings.HasPrefix(r.input.StartURLs[0].URL, "https://boston.craigslist.org/search/apa?") {
		t.Errorf("input = %+v", r.input)
	}
	if len(got) != 3 {
		t.Fatalf("got %d listings", len(got))
	}

	l := got[0]
	if l.Source != listing.SourceCraigslist || l.SourceID != "j2QRLoSwSfTzCLCkKJx9Q9" || l.Offer != listing.OfferRent ||
		l.Price.Cents != 350000 || *l.Beds != 2 || *l.Baths != 1 || l.SqFt != nil {
		t.Errorf("listing = %+v", l)
	}
	if l.Address.Formatted != "Benton Rd, Tufts University" {
		t.Errorf("address = %q", l.Address.Formatted)
	}
	if l.Coordinates == nil || l.Coordinates.Lat != 42.403933 || l.Coordinates.Lng != -71.110973 || l.ListedAt == nil || !l.ListedAt.Equal(time.Date(2026, 9, 29, 23, 30, 0, 0, time.UTC)) {
		t.Errorf("coords/listed = %+v %v", l.Coordinates, l.ListedAt)
	}
	if len(l.Photos) != 3 || !strings.HasSuffix(l.Photos[0], "_1200x900.jpg") {
		t.Errorf("photos = %v", l.Photos)
	}
	if !l.Amenities[listing.AmenityInUnitLaundry] || !l.Amenities[listing.AmenityParking] {
		t.Errorf("amenities = %v", l.Amenities)
	}
	if !strings.HasPrefix(l.Description, ">>MEDFORD/TUFTS 2BED") || !strings.Contains(l.Description, "PROPERTY INFO") {
		t.Errorf("description = %.60q", l.Description)
	}

	davis := got[1]
	if davis.Address.Formatted != "Morrison Ave, Somerville / Davis Sq." {
		t.Errorf("location segments should be deduplicated: %q", davis.Address.Formatted)
	}
	if davis.Amenities[listing.AmenityInUnitLaundry] || davis.Amenities[listing.AmenityParking] != true {
		t.Errorf("laundry on site is not in-unit; off-street parking is parking: %v", davis.Amenities)
	}
	if teele := got[2]; teele.Amenities[listing.AmenityParking] {
		t.Errorf("street parking should not count: %v", teele.Amenities)
	}
}

func TestSearchPostFilters(t *testing.T) {
	p, _ := newProvider(fixture(t), zip02144)
	got, err := p.Search(context.Background(), listing.Query{
		Offer:     listing.OfferRent,
		Area:      listing.Area{Location: "02144"},
		MaxAge:    2 * time.Hour,
		Amenities: []listing.Amenity{listing.AmenityInUnitLaundry},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("listings older than max age should be dropped, got %d", len(got))
	}

	near, _ := newProvider(fixture(t), zillow.Region{Name: "Davis", Type: "neighborhood", Center: listing.Coordinates{Lat: 42.3967, Lng: -71.1225}})
	got, err = near.Search(context.Background(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Davis Square", RadiusMiles: 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got {
		if l.SourceID == "j2QRLoSwSfTzCLCkKJx9Q9" {
			t.Error("Medford listing is outside a 0.5 mile radius of Davis Square")
		}
	}
}

func TestSearchRejectsUnsupported(t *testing.T) {
	p, _ := newProvider(nil, zip02144)
	for _, q := range []listing.Query{
		{Offer: listing.OfferRent, Area: listing.Area{Location: "02144"}, Amenities: []listing.Amenity{listing.AmenityDishwasher}},
		{Offer: listing.OfferSale, Area: listing.Area{Location: "02144"}},
		{Offer: listing.OfferRent},
	} {
		if _, err := p.Search(context.Background(), q); !errors.Is(err, listing.ErrUnsupportedQuery) {
			t.Errorf("%+v: got %v", q, err)
		}
	}
}

func TestToListingSkipsUnpriced(t *testing.T) {
	_, ok, err := toListing(json.RawMessage(`{"id":"x","url":"u","price":""}`), now)
	if err != nil || ok {
		t.Errorf("ok=%v err=%v", ok, err)
	}
	l, ok, _ := toListing(json.RawMessage(`{"id":"x","url":"u","price":"$1,200","space":"850ft2","bathrooms":"1.5"}`), now)
	if !ok || l.Offer != listing.OfferRent || *l.SqFt != 850 || *l.Baths != 1.5 || l.ObservedAt != now {
		t.Errorf("listing = %+v", l)
	}
}

func TestSupportedAmenitiesRentOnly(t *testing.T) {
	p, _ := newProvider(nil, zip02144)
	if len(p.SupportedAmenities(listing.OfferRent)) != 3 || p.SupportedAmenities(listing.OfferSale) != nil {
		t.Errorf("rent=%v sale=%v", p.SupportedAmenities(listing.OfferRent), p.SupportedAmenities(listing.OfferSale))
	}
}
