package redfin

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func lookupProvider(t *testing.T, detail, search []json.RawMessage) (*Provider, *[]string, *actorInput) {
	p, _ := newProvider(nil)
	var actors []string
	var searched actorInput
	p.Runner = runnerFunc(func(actor string, in any) ([]json.RawMessage, error) {
		actors = append(actors, actor)
		switch actor {
		case DetailActorID:
			return detail, nil
		case ActorID:
			searched = in.(actorInput)
			return search, nil
		}
		t.Fatalf("unexpected actor %s", actor)
		return nil, nil
	})
	return p, &actors, &searched
}

func TestListingURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201?utm=x#photos":    "https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201",
		"https://redfin.com/MA/Somerville/296-Highland-Ave-02144/unit-2/apartment/178268007/": "https://www.redfin.com/MA/Somerville/296-Highland-Ave-02144/unit-2/apartment/178268007",
		"http://WWW.REDFIN.COM/NY/Jamaica/Ruby-Square/apartment/186342666":                    "https://www.redfin.com/NY/Jamaica/Ruby-Square/apartment/186342666",
	} {
		if got, err := ListingURL(in); err != nil || got != want {
			t.Errorf("ListingURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"https://www.redfin.com/zipcode/02144/apartments-for-rent",
		"https://www.redfin.com/city/16169/MA/Somerville",
		"https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/",
		"https://www.zillow.com/MA/Somerville/91-Heath-St-02145/home/8714201",
	} {
		if _, err := ListingURL(in); err == nil {
			t.Errorf("ListingURL(%q) should fail", in)
		}
	}
}

func TestLookupSale(t *testing.T) {
	searchP, _ := newProvider(fixture(t, "sale.json"))
	found, err := searchP.Search(t.Context(), listing.Query{Offer: listing.OfferSale, Area: listing.Area{Location: "02144"}})
	if err != nil {
		t.Fatal(err)
	}
	fromSearch := found[0]

	p, actors, _ := lookupProvider(t, fixture(t, "detail.json"), nil)
	var input detailInput
	inner := p.Runner
	p.Runner = runnerFunc(func(actor string, in any) ([]json.RawMessage, error) {
		input = in.(detailInput)
		return inner.Run(t.Context(), actor, in)
	})
	got, err := p.Lookup(t.Context(), "https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201?utm_source=x")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*actors, []string{DetailActorID}) {
		t.Errorf("actors = %v", *actors)
	}
	if len(input.DetailURLs) != 1 || input.DetailURLs[0].URL != "https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201" {
		t.Errorf("detail input = %+v", input)
	}
	if got.Source != fromSearch.Source || got.SourceID != fromSearch.SourceID || got.URL != fromSearch.URL {
		t.Errorf("identity %s/%s %s, search has %s/%s %s", got.Source, got.SourceID, got.URL, fromSearch.Source, fromSearch.SourceID, fromSearch.URL)
	}
	if got.Offer != listing.OfferSale || got.Price != fromSearch.Price || *got.Beds != 3 || *got.Baths != 1 || *got.SqFt != 1049 ||
		got.Address != fromSearch.Address || *got.Coordinates != *fromSearch.Coordinates || got.ListedAt == nil || !got.ObservedAt.Equal(now) {
		t.Errorf("sale = %+v\nsearch = %+v", got, fromSearch)
	}
	if len(got.Description) < 1000 || len(got.Photos) != 3 || !strings.Contains(got.Photos[0], "/bigphoto/") || len(got.Raw) == 0 {
		t.Errorf("details: %d chars, photos %v", len(got.Description), got.Photos)
	}
	if !got.Amenities[listing.AmenityInUnitLaundry] || !got.Amenities[listing.AmenityParking] {
		t.Errorf("amenities = %v", got.Amenities)
	}
}

func TestLookupRental(t *testing.T) {
	searchP, _ := newProvider(fixture(t, "rent.json"))
	found, err := searchP.Search(t.Context(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "02144"}})
	if err != nil {
		t.Fatal(err)
	}
	fromSearch := found[0]

	p, actors, searched := lookupProvider(t, fixture(t, "detail.json"), fixture(t, "rent.json"))
	got, err := p.Lookup(t.Context(), "https://www.redfin.com/MA/Somerville/119-College-Ave-02144/apartment/8694910")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*actors, []string{DetailActorID, ActorID}) {
		t.Errorf("actors = %v", *actors)
	}
	u := searched.SearchURLs[0].URL
	if searched.ZoomIn || searched.MaxResults != rentalLookupLimit || !strings.HasPrefix(u, "https://www.redfin.com/zipcode/02144/apartments-for-rent/filter/viewport=42.40") || !strings.HasSuffix(u, ",no-outline") {
		t.Errorf("search input = %+v", *searched)
	}
	if got.SourceID != fromSearch.SourceID || got.URL != fromSearch.URL || got.SourceID != "1bcb9204-7ae9-45a0-b607-8f6eaa071e07" {
		t.Errorf("identity %s %s, search has %s %s", got.SourceID, got.URL, fromSearch.SourceID, fromSearch.URL)
	}
	if got.Offer != listing.OfferRent || got.Price.Cents != 260000 || got.Address != fromSearch.Address || len(got.Photos) != 87 || got.Coordinates == nil {
		t.Errorf("rental = %+v", got)
	}
}

func TestLookupErrors(t *testing.T) {
	ctx := t.Context()
	heath := "https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201"
	for name, tc := range map[string]struct {
		url            string
		detail, search []json.RawMessage
		want           string
	}{
		"search page":   {url: "https://www.redfin.com/zipcode/02144", want: "not a redfin listing page"},
		"empty output":  {url: heath, want: "returned nothing"},
		"garbage":       {url: heath, detail: []json.RawMessage{json.RawMessage(`{"foo":1}`), json.RawMessage(`{"ok":false,"error":"blocked"}`)}, want: "returned nothing"},
		"not json":      {url: heath, detail: []json.RawMessage{json.RawMessage(`"oops"`)}, want: "decode redfin detail"},
		"other listing": {url: "https://www.redfin.com/MA/Somerville/1-Elsewhere-St-02144/home/1", detail: fixture(t, "detail.json"), want: "returned nothing"},
		"not on market": {url: "https://www.redfin.com/MA/Somerville/296-Highland-Ave-02144/unit-2/apartment/178268007", detail: fixture(t, "detail.json"), search: fixture(t, "sale.json"), want: "not for sale or rent"},
	} {
		p, _, _ := lookupProvider(t, tc.detail, tc.search)
		if _, err := p.Lookup(ctx, tc.url); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
