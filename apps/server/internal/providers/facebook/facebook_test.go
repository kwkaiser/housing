package facebook

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

type fakeRunner struct {
	items []json.RawMessage
	input any
	actor string
}

func (f *fakeRunner) Run(_ context.Context, actor string, input any) ([]json.RawMessage, error) {
	f.actor, f.input = actor, input
	return f.items, nil
}

type fakeRegions struct{ region zillow.Region }

func (f fakeRegions) Resolve(context.Context, string) (zillow.Region, error) { return f.region, nil }

var (
	somerville = zillow.Region{Name: "Somerville, MA", Type: "city", Center: listing.Coordinates{Lat: 42.3876, Lng: -71.0995}}
	now        = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
)

func fixture(t *testing.T) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/search.json")
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

func ptr[T any](v T) *T { return &v }

func TestSearchURL(t *testing.T) {
	q := listing.Query{
		Offer:    listing.OfferRent,
		MinPrice: &listing.Money{Cents: 100000},
		MaxPrice: &listing.Money{Cents: 400050},
		MinBeds:  ptr(1),
		MaxBeds:  ptr(3),
		MaxAge:   5 * day,
	}
	u, err := url.Parse(searchURL(q, somerville.Center, 5))
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/marketplace/boston/propertyrentals" {
		t.Errorf("path = %s", u.Path)
	}
	want := map[string]string{
		"latitude": "42.38760", "longitude": "-71.09950", "radius": "9",
		"minPrice": "1000", "maxPrice": "4001", "minBedrooms": "1", "maxBedrooms": "3",
		"daysSinceListed": "7", "sortBy": "creation_time_descend",
	}
	for k, v := range want {
		if got := u.Query().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, ok := daysSinceListed(60 * day); ok {
		t.Error("ages beyond 30 days should not filter")
	}
}

func TestSearch(t *testing.T) {
	p, r := newProvider(fixture(t), somerville)
	got, err := p.Search(t.Context(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Somerville, MA"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	ri, ok := r.input.(apify.RunInput)
	if !ok || ri.MaxItems != 10 || r.actor != ActorID {
		t.Fatalf("runner input = %#v", r.input)
	}
	in := ri.Input.(actorInput)
	if !in.GetListingDetails || !in.GetAllListingPhotos || len(in.URLs) != 1 || !strings.Contains(in.URLs[0], "radius=9") {
		t.Errorf("actor input = %+v", in)
	}

	ids := map[string]listing.Listing{}
	for _, l := range got {
		ids[l.SourceID] = l
	}
	if len(got) != 2 {
		t.Errorf("got %d listings %v, want the two within 5 miles (parking and Lawrence dropped)", len(got), ids)
	}
	if _, ok := ids["1711690079891733"]; ok {
		t.Error("listings without bedroom info should be dropped")
	}
	if _, ok := ids["2642401882860144"]; ok {
		t.Error("Lawrence is outside a 5 mile radius of Somerville")
	}

	allston := ids["1071136835630251"]
	if allston.Offer != listing.OfferRent || allston.Price.Cents != 235000 || *allston.Beds != 2 || *allston.Baths != 1 ||
		allston.Coordinates == nil || allston.ListedAt == nil || len(allston.Photos) != 4 || allston.Description == "" {
		t.Errorf("allston = %+v", allston)
	}
	if allston.Address.Formatted != "75 Gardner St, Allston, MA 02134" || allston.Address.PostalCode != "02134" {
		t.Errorf("address = %+v", allston.Address)
	}
	if allston.URL != "https://www.facebook.com/marketplace/item/1071136835630251" {
		t.Errorf("url = %s", allston.URL)
	}

	room := ids["1146320321403911"]
	if room.Address.Formatted != "Somerville, MA 02145" || *room.Beds != 1 {
		t.Errorf("room = %+v", room)
	}
}

func TestSearchRejects(t *testing.T) {
	p, _ := newProvider(nil, somerville)
	for _, q := range []listing.Query{
		{Offer: listing.OfferSale, Area: listing.Area{Location: "x"}},
		{Offer: listing.OfferRent},
		{Offer: listing.OfferRent, Area: listing.Area{Location: "x"}, Amenities: []listing.Amenity{listing.AmenityDishwasher}},
	} {
		if _, err := p.Search(t.Context(), q); !errors.Is(err, listing.ErrUnsupportedQuery) {
			t.Errorf("%+v: got %v", q, err)
		}
	}
	state, _ := newProvider(nil, zillow.Region{Name: "MA", Type: "state"})
	if _, err := state.Search(t.Context(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "MA"}}); !errors.Is(err, listing.ErrUnsupportedQuery) {
		t.Errorf("state without radius: %v", err)
	}
}

func TestSearchMaxAgePostFilter(t *testing.T) {
	p, _ := newProvider(fixture(t), somerville)
	p.Now = func() time.Time { return time.Unix(1790735658, 0).Add(2 * time.Hour) }
	got, err := p.Search(t.Context(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Somerville, MA"}, MaxAge: 3 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got {
		if l.SourceID != "1071136835630251" {
			t.Errorf("%s is older than 3 hours", l.SourceID)
		}
	}
}

func TestParsers(t *testing.T) {
	beds := map[string]int{"2 beds · 1 bath": 2, "Studio 1 Bath - Apartment": 0, "Private room for rent": 1, "3BR apartment": 3}
	for s, want := range beds {
		if got := parseBeds(s); got == nil || *got != want {
			t.Errorf("parseBeds(%q) = %v, want %d", s, got, want)
		}
	}
	if parseBeds("ACL Parking") != nil {
		t.Error("parking has no bedrooms")
	}
	if b := parseBaths("3 Beds 2.5 Baths House"); b == nil || *b != 2.5 {
		t.Errorf("parseBaths = %v", b)
	}
	if s := parseSqFt("Spacious 1,050 sq ft unit"); s == nil || *s != 1050 {
		t.Errorf("parseSqFt = %v", s)
	}
}
