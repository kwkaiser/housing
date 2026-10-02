package streeteasy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

type fakeRunner struct {
	mu     sync.Mutex
	items  []json.RawMessage
	inputs []apify.RunInput
}

func (f *fakeRunner) Run(_ context.Context, _ string, input any) ([]json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ri, ok := input.(apify.RunInput)
	if !ok {
		ri = apify.RunInput{Input: input}
	}
	f.inputs = append(f.inputs, ri)
	return f.items, nil
}

func (f *fakeRunner) urls() []string {
	var out []string
	for _, ri := range f.inputs {
		for _, u := range ri.Input.(actorInput).StartURLs {
			out = append(out, u.URL)
		}
	}
	slices.Sort(out)
	return out
}

type fakeRegions struct {
	region zillow.Region
	err    error
	query  string
}

func (f *fakeRegions) Resolve(_ context.Context, q string) (zillow.Region, error) {
	f.query = q
	return f.region, f.err
}

var (
	now          = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	williamsburg = zillow.Region{Name: "Williamsburg, Brooklyn, NY", Type: "neighborhood", Center: listing.Coordinates{Lat: 40.7145, Lng: -73.9425}}
)

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

func newProvider(items []json.RawMessage, region zillow.Region) (*Provider, *fakeRunner, *fakeRegions) {
	r := &fakeRunner{items: items}
	regions := &fakeRegions{region: region}
	p := New(r, regions)
	p.Now = func() time.Time { return now }
	return p, r, regions
}

func ptr[T any](v T) *T { return &v }

func TestBuildSearchURL(t *testing.T) {
	cases := []struct {
		name string
		q    listing.Query
		want string
	}{
		{
			"bare rental",
			listing.Query{Offer: listing.OfferRent},
			"https://streeteasy.com/for-rent/nyc/zip:11211?sort_by=listed_desc",
		},
		{
			"rental filters",
			listing.Query{
				Offer:     listing.OfferRent,
				MinPrice:  &listing.Money{Cents: 200000},
				MaxPrice:  &listing.Money{Cents: 400050},
				MinBeds:   ptr(1),
				MaxBeds:   ptr(2),
				Amenities: []listing.Amenity{listing.AmenityParking, listing.AmenityInUnitLaundry},
			},
			"https://streeteasy.com/for-rent/nyc/zip:11211%7Cprice:2000-4001%7Cbeds:1-2%7Camenities:parking,washer_dryer?sort_by=listed_desc",
		},
		{
			"sale open ranges",
			listing.Query{Offer: listing.OfferSale, MaxPrice: &listing.Money{Cents: 150000000}, MinBeds: ptr(2)},
			"https://streeteasy.com/for-sale/nyc/zip:11211%7Cprice:-1500000%7Cbeds%3E=2?sort_by=listed_desc",
		},
		{
			"max beds only",
			listing.Query{Offer: listing.OfferRent, MinPrice: &listing.Money{Cents: 300000}, MaxBeds: ptr(1)},
			"https://streeteasy.com/for-rent/nyc/zip:11211%7Cprice:3000-%7Cbeds:0-1?sort_by=listed_desc",
		},
	}
	for _, tc := range cases {
		if got := buildSearchURL(tc.q, "11211"); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, got, tc.want)
		}
	}
}

func TestSearchRentRadius(t *testing.T) {
	p, r, _ := newProvider(fixture(t, "rent.json"), williamsburg)
	got, err := p.Search(t.Context(), listing.Query{
		Offer: listing.OfferRent,
		Area:  listing.Area{Location: "Williamsburg, Brooklyn", RadiusMiles: 0.6},
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	urls := r.urls()
	if len(urls) < 2 {
		t.Fatalf("searched %v, want one run per nearby zip", urls)
	}
	for _, zip := range []string{"11206", "11211"} {
		if !slices.ContainsFunc(urls, func(u string) bool { return strings.Contains(u, "zip:"+zip+"?") }) {
			t.Errorf("searched %v, missing zip %s", urls, zip)
		}
	}
	for _, ri := range r.inputs {
		in := ri.Input.(actorInput)
		if len(in.StartURLs) != 1 || in.MaxItems != ri.MaxItems || in.MaxItems < 1 || in.FlattenDatasetItems || in.MonitoringMode {
			t.Errorf("run input = %+v", ri)
		}
	}

	var ids []string
	for _, l := range got {
		ids = append(ids, l.SourceID)
	}
	if want := []string{"5165844", "5147149"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v (deduped, newest first, 5170579 outside radius)", ids, want)
	}
}

func TestSearchSingleZip(t *testing.T) {
	p, r, regions := newProvider(fixture(t, "rent.json"), williamsburg)
	got, err := p.Search(t.Context(), listing.Query{
		Offer:  listing.OfferRent,
		Area:   listing.Area{Location: "11206"},
		MaxAge: 10 * 24 * time.Hour,
		Limit:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if urls := r.urls(); len(urls) != 1 || !strings.Contains(urls[0], "zip:11206?") || r.inputs[0].MaxItems != 3 {
		t.Errorf("runs = %+v", r.inputs)
	}
	if regions.query != "" {
		t.Errorf("zip search should not resolve a region, resolved %q", regions.query)
	}
	if len(got) != 2 || got[0].SourceID != "5170579" || got[1].SourceID != "5165844" {
		t.Errorf("got %d listings %+v, want the two listed within 10 days", len(got), got)
	}
}

func TestSearchZipRadiusUsesTable(t *testing.T) {
	p, r, regions := newProvider(nil, williamsburg)
	if _, err := p.Search(t.Context(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "11211", RadiusMiles: 1}}); err != nil {
		t.Fatal(err)
	}
	if regions.query != "" || len(r.inputs) < 3 {
		t.Errorf("resolved %q, ran %d searches", regions.query, len(r.inputs))
	}
	if !strings.HasPrefix(r.urls()[0], "https://streeteasy.com/for-sale/") {
		t.Errorf("urls = %v", r.urls())
	}
}

func TestSearchRejects(t *testing.T) {
	boston := zillow.Region{Name: "Somerville, MA", Type: "city", Center: listing.Coordinates{Lat: 42.3876, Lng: -71.0995}}
	cases := []struct {
		name   string
		q      listing.Query
		region zillow.Region
		want   string
	}{
		{"offer", listing.Query{Offer: "lease", Area: listing.Area{Location: "11211"}}, williamsburg, "offer type"},
		{"location", listing.Query{Offer: listing.OfferRent}, williamsburg, "location is required"},
		{"amenity", listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "11211"}, Amenities: []listing.Amenity{listing.AmenityAirConditioning}}, williamsburg, "cannot filter by air_conditioning"},
		{"outside nyc", listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Somerville, MA", RadiusMiles: 2}}, boston, "only covers the New York City area"},
		{"no radius for city", listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Brooklyn"}}, zillow.Region{Name: "Brooklyn", Type: "city", Center: williamsburg.Center}, "set a radius"},
		{"too many zips", listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Manhattan", RadiusMiles: 3}}, zillow.Region{Name: "Manhattan", Type: "city", Center: listing.Coordinates{Lat: 40.7359, Lng: -73.9911}}, "zip codes on streeteasy"},
	}
	for _, tc := range cases {
		p, r, _ := newProvider(nil, tc.region)
		_, err := p.Search(t.Context(), tc.q)
		if !errors.Is(err, listing.ErrUnsupportedQuery) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
		if len(r.inputs) != 0 {
			t.Errorf("%s: ran the actor", tc.name)
		}
	}
}

func TestToListingRental(t *testing.T) {
	l, ok, err := toListing(fixture(t, "rent.json")[1], listing.OfferRent, now)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if l.Source != listing.SourceStreetEasy || l.SourceID != "5165844" || l.URL != "https://streeteasy.com/building/240-meeker-avenue-brooklyn/9" || l.Offer != listing.OfferRent {
		t.Errorf("identity = %+v", l)
	}
	if l.Price != (listing.Money{Cents: 399900, Currency: "USD"}) || *l.Beds != 1 || *l.Baths != 1 || l.SqFt != nil {
		t.Errorf("facts = price %+v beds %v baths %v sqft %v", l.Price, *l.Beds, *l.Baths, l.SqFt)
	}
	want := listing.Address{Formatted: "240 Meeker Avenue #9, Brooklyn, NY 11211", Street: "240 Meeker Avenue", Unit: "9", City: "Brooklyn", State: "NY", PostalCode: "11211"}
	if l.Address != want {
		t.Errorf("address = %+v", l.Address)
	}
	if l.Coordinates == nil || l.Coordinates.Lat != 40.71581 || l.Coordinates.Lng != -73.95034 {
		t.Errorf("coordinates = %+v", l.Coordinates)
	}
	if l.ListedAt == nil || !l.ListedAt.Equal(time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("listed at = %v", l.ListedAt)
	}
	if len(l.Photos) != 9 || !strings.HasSuffix(l.Photos[0], "-full.webp") {
		t.Errorf("photos = %v", l.Photos)
	}
	if !l.Amenities[listing.AmenityDishwasher] || !l.Amenities[listing.AmenityAirConditioning] || l.Amenities[listing.AmenityParking] {
		t.Errorf("amenities = %v", l.Amenities)
	}
	if !strings.HasPrefix(l.Description, "Super bright") || len(l.Raw) == 0 {
		t.Errorf("description = %q", l.Description)
	}
}

func TestToListingSale(t *testing.T) {
	l, ok, err := toListing(fixture(t, "sale.json")[0], listing.OfferSale, now)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if l.Offer != listing.OfferSale || l.Price.Cents != 97500000 || *l.Beds != 2 || l.Address.Unit != "12" || l.Address.PostalCode != "11215" {
		t.Errorf("sale = %+v", l)
	}
	if !l.Amenities[listing.AmenityDishwasher] {
		t.Errorf("amenities = %v", l.Amenities)
	}
}

func TestToListingSkipsUnpriced(t *testing.T) {
	for _, raw := range []string{`{"id":"1","urlPath":"/building/x/1"}`, `{"urlPath":"/building/x/1","price":100}`, `{"id":"1","price":100}`} {
		if _, ok, err := toListing(json.RawMessage(raw), listing.OfferRent, now); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", raw, ok, err)
		}
	}
	if _, _, err := toListing(json.RawMessage(`[]`), listing.OfferRent, now); err == nil {
		t.Error("want decode error")
	}
}
