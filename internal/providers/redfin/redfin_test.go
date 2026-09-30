package redfin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
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

var somerville = zillow.Region{Name: "Somerville, MA", Type: "city", Center: listing.Coordinates{Lat: 42.3876, Lng: -71.0995}}

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []json.RawMessage {
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

func newProvider(items []json.RawMessage) (*Provider, *fakeRunner) {
	r := &fakeRunner{items: items}
	p := New(r, fakeRegions{somerville})
	p.Now = func() time.Time { return now }
	return p, r
}

func ptr[T any](v T) *T { return &v }

func TestBuildSearchURL(t *testing.T) {
	cases := []struct {
		name string
		q    listing.Query
		a    searchArea
		want string
	}{
		{
			"zip rental",
			listing.Query{Offer: listing.OfferRent, MaxPrice: &listing.Money{Cents: 400000}, MinBeds: ptr(1)},
			searchArea{zip: "02144", outline: true},
			"https://www.redfin.com/zipcode/02144/apartments-for-rent/filter/max-price=4000,min-beds=1",
		},
		{
			"viewport sale",
			listing.Query{Offer: listing.OfferSale, MinPrice: &listing.Money{Cents: 50000000}, MaxAge: 10 * day},
			searchArea{zip: viewportAnchorZip, viewport: true, bounds: bounds(42.4, 42.3, -71.0, -71.2)},
			"https://www.redfin.com/zipcode/10001/filter/min-price=500000,max-days-on-market=2wk,viewport=42.40000:42.30000:-71.00000:-71.20000,no-outline",
		},
		{
			"rental ignores age",
			listing.Query{Offer: listing.OfferRent, MaxAge: 3 * day},
			searchArea{zip: "02144", outline: true},
			"https://www.redfin.com/zipcode/02144/apartments-for-rent",
		},
	}
	for _, c := range cases {
		if got := buildSearchURL(c.q, c.a); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestResolve(t *testing.T) {
	p, _ := newProvider(nil)
	zip, err := p.resolve(context.Background(), listing.Area{Location: "02144"})
	if err != nil || !zip.outline || zip.zip != "02144" || zip.viewport {
		t.Errorf("plain zip should use redfin's own boundary: %+v %v", zip, err)
	}
	radius, err := p.resolve(context.Background(), listing.Area{Location: "02144", RadiusMiles: 1})
	if err != nil || !radius.viewport || radius.radius != 1 || radius.zip != "02144" {
		t.Errorf("zip with radius should use a viewport: %+v %v", radius, err)
	}
	city, err := p.resolve(context.Background(), listing.Area{Location: "Somerville, MA"})
	if err != nil || !city.viewport || city.radius != 0 || city.zip != viewportAnchorZip {
		t.Errorf("city should use a viewport around its center: %+v %v", city, err)
	}

	state := New(&fakeRunner{}, fakeRegions{zillow.Region{Name: "MA", Type: "state"}})
	if _, err := state.resolve(context.Background(), listing.Area{Location: "MA"}); !errors.Is(err, listing.ErrUnsupportedQuery) {
		t.Errorf("state without radius should be rejected: %v", err)
	}
}

func TestSearchRentals(t *testing.T) {
	p, r := newProvider(fixture(t, "rent.json"))
	got, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "02144"}, MinBeds: ptr(1), Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.MaxResults != 5 || !strings.Contains(r.input.SearchURLs[0].URL, "/zipcode/02144/apartments-for-rent") {
		t.Errorf("input = %+v", r.input)
	}
	if len(got) != 3 {
		t.Fatalf("got %d listings", len(got))
	}
	l := got[0]
	if l.Source != listing.SourceRedfin || l.SourceID != "1bcb9204-7ae9-45a0-b607-8f6eaa071e07" || l.Offer != listing.OfferRent ||
		l.Price.Cents != 260000 || *l.Beds != 1 || *l.SqFt != 553 || l.Coordinates == nil || l.Description == "" {
		t.Errorf("rental = %+v", l)
	}
	if l.Address.Formatted != "119 College Ave, Somerville, MA 02144" {
		t.Errorf("address = %q", l.Address.Formatted)
	}
	if len(l.Photos) != 87 || l.Photos[0] != photoBase+"/rent/1bcb9204-7ae9-45a0-b607-8f6eaa071e07/bigphoto/0_5.jpg" || !strings.HasSuffix(l.Photos[10], "/10_4.jpg") {
		t.Errorf("photos = %d %v", len(l.Photos), l.Photos[:2])
	}
	if l.ListedAt != nil {
		t.Error("rentals have no listing date")
	}
}

func TestSearchSales(t *testing.T) {
	p, _ := newProvider(fixture(t, "sale.json"))
	got, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "02144"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d listings", len(got))
	}
	heath := got[0]
	if heath.SourceID != "8714201" || heath.Price.Cents != 73900000 || *heath.Beds != 3 || *heath.Baths != 1 || *heath.SqFt != 1049 {
		t.Errorf("sale = %+v", heath)
	}
	if heath.ListedAt == nil || !heath.ListedAt.Equal(now.Add(-day)) {
		t.Errorf("listed at = %v", heath.ListedAt)
	}
	if len(heath.Photos) != 36 || heath.Photos[0] != photoBase+"/52/bigphoto/920/73583920_0.jpg" || heath.Photos[1] != photoBase+"/52/bigphoto/920/73583920_1_0.jpg" {
		t.Errorf("photos = %v", heath.Photos[:2])
	}
	if !strings.HasPrefix(heath.Description, "Welcome to 91 Heath Street") {
		t.Errorf("description = %.40q", heath.Description)
	}

	ship := got[1]
	if ship.Address.Unit != "34" || ship.Address.Street != "20 Ship Ave" || len(ship.Photos) != 0 {
		t.Errorf("unit address / missing photos: %+v", ship.Address)
	}
	cross := got[2]
	if cross.Photos[0] != photoBase+"/641/bigphoto/409/2200414394766808409_0.jpg" {
		t.Errorf("photo dir should be the last three digits of the listing id: %s", cross.Photos[0])
	}
}

func TestSearchRadiusFiltersByDistance(t *testing.T) {
	p, _ := newProvider(fixture(t, "sale.json"))
	got, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "Somerville, MA", RadiusMiles: 1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got {
		if l.Address.City == "Medford" {
			t.Errorf("Medford listing is outside a 1 mile radius of Somerville: %s", l.Address.Formatted)
		}
	}
}

func TestSearchRejectsAmenities(t *testing.T) {
	p, _ := newProvider(nil)
	_, err := p.Search(context.Background(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "02144"}, Amenities: []listing.Amenity{listing.AmenityDishwasher}})
	if !errors.Is(err, listing.ErrUnsupportedQuery) {
		t.Fatalf("got %v", err)
	}
}

func TestSaleAmenitiesFromTags(t *testing.T) {
	raw := json.RawMessage(`{"url":"u","propertyId":1,"offerType":"sale","price":{"value":1},"listingTags":["IN-UNIT LAUNDRY","CENTRAL AIR"],"listingRemarks":"Renovated kitchen with dishwasher."}`)
	l, ok, err := toListing(raw, listing.Query{}, now)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	for _, a := range []listing.Amenity{listing.AmenityInUnitLaundry, listing.AmenityAirConditioning, listing.AmenityDishwasher} {
		if !l.Amenities[a] {
			t.Errorf("missing %s: %v", a, l.Amenities)
		}
	}
}

func bounds(n, s, e, w float64) geo.Bounds { return geo.Bounds{North: n, South: s, East: e, West: w} }

func TestEnrichSales(t *testing.T) {
	sales, _ := newProvider(fixture(t, "sale.json"))
	found, err := sales.Search(context.Background(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "02144"}})
	if err != nil {
		t.Fatal(err)
	}
	rent := listing.Listing{Source: listing.SourceRedfin, SourceID: "r", URL: "https://www.redfin.com/MA/Somerville/296-Highland-Ave-02144/unit-2/apartment/178268007", Offer: listing.OfferRent, Description: "rental"}
	zillowListing := listing.Listing{Source: listing.SourceZillow, SourceID: "z", URL: "https://www.zillow.com/x", Offer: listing.OfferSale}
	in := append(found, rent, zillowListing)

	p, r := newProvider(fixture(t, "detail.json"))
	var input detailInput
	p.Runner = runnerFunc(func(actor string, in any) ([]json.RawMessage, error) {
		if actor != DetailActorID {
			t.Errorf("actor = %s", actor)
		}
		input = in.(detailInput)
		return r.items, nil
	})
	got, err := p.Enrich(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.DetailURLs) != 3 {
		t.Errorf("only redfin sales should be enriched, got %d urls", len(input.DetailURLs))
	}
	if len(got) != len(in) {
		t.Fatalf("got %d listings", len(got))
	}

	heath := got[0]
	if len(heath.Description) < 1000 || strings.Contains(heath.Description, "&rsquo;") {
		t.Errorf("description should be the full, unescaped remarks: %d chars", len(heath.Description))
	}
	if !heath.Amenities[listing.AmenityInUnitLaundry] || !heath.Amenities[listing.AmenityParking] {
		t.Errorf("amenities = %v", heath.Amenities)
	}
	if len(heath.Photos) != 3 || heath.Photos[1] != photoBase+"/52/bigphoto/920/73583920_1_0.jpg" {
		t.Errorf("photos = %v", heath.Photos)
	}
	if got[3].Description != "rental" || got[4].Source != listing.SourceZillow {
		t.Error("rentals and other sources should pass through unchanged")
	}
}

func TestAmenityFromFact(t *testing.T) {
	cases := map[[2]string]listing.Amenity{
		{"Laundry", "In-unit laundry (washer and dryer)"}: listing.AmenityInUnitLaundry,
		{"Parking", "2 spaces"}:                           listing.AmenityParking,
		{"Cooling", "Central air"}:                        listing.AmenityAirConditioning,
		{"Appliances", "Dishwasher, Range"}:               listing.AmenityDishwasher,
	}
	for in, want := range cases {
		if got := amenityFromFact(in[0], in[1]); !got[want] {
			t.Errorf("%v: got %v", in, got)
		}
	}
	for _, in := range [][2]string{{"Parking", "Street parking"}, {"Parking", "0 spaces"}, {"Laundry", "Laundry in building"}, {"Cooling", "None"}} {
		if got := amenityFromFact(in[0], in[1]); len(got) != 0 {
			t.Errorf("%v should not count: %v", in, got)
		}
	}
}

type runnerFunc func(actor string, input any) ([]json.RawMessage, error)

func (f runnerFunc) Run(_ context.Context, actor string, input any) ([]json.RawMessage, error) {
	return f(actor, input)
}
